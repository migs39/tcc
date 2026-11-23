// Comando cdn: guarda os livros cifrados e os entrega mediante token cego
// válido e não usado (RF17); reporta os tokens usados ao site em lotes (RF18).
//
// Variáveis de ambiente:
//
//	DATABASE_URL     Postgres da CDN (padrão: Postgres do docker-compose.yml,
//	                 schema "cdn", separado das tabelas do site)
//	ENDERECO         endereço de escuta (padrão: localhost:8081)
//	SITE_URL         endereço do site (padrão: http://localhost:8080)
//	SEGREDO_CDN      segredo compartilhado com o site (obrigatório)
//	CHAVE_PACOTE_PUB chave pública Ed25519 do pacote do catálogo, em hex
//	                 (obrigatória; `go run ./cmd/chave-pacote` no site)
//	PASTA_LIVROS     onde ficam os livros cifrados (padrão: livros_cifrados)
//	LOTE_MINIMO      tokens por lote do reporte (padrão: 50, D-011)
//	INTERVALO_LOTE   intervalo máximo entre lotes (padrão: 24h, D-011)
package main

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"tcc/cdn/internal/cdn"
)

const urlPadrao = "postgres://livraria:livraria_dev@localhost:5432/livraria?sslmode=disable&search_path=cdn"

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	if err := rodar(log); err != nil {
		log.Error("cdn encerrada com erro", "erro", err)
		os.Exit(1)
	}
}

func rodar(log *slog.Logger) error {
	ctx, parar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer parar()

	segredo := os.Getenv("SEGREDO_CDN")
	if segredo == "" {
		return errors.New("defina SEGREDO_CDN (o mesmo do site)")
	}
	pubHex := os.Getenv("CHAVE_PACOTE_PUB")
	pub, err := hex.DecodeString(pubHex)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return errors.New("defina CHAVE_PACOTE_PUB com a chave pública do pacote (hex)")
	}

	pool, err := pgxpool.New(ctx, getenv("DATABASE_URL", urlPadrao))
	if err != nil {
		return err
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS cdn`); err != nil {
		return err
	}
	if err := cdn.Migrar(ctx, pool); err != nil {
		return err
	}

	lote, _ := strconv.Atoi(getenv("LOTE_MINIMO", "50"))
	intervalo, err := time.ParseDuration(getenv("INTERVALO_LOTE", "24h"))
	if err != nil {
		return err
	}
	c := cdn.Nova(pool, cdn.Config{
		Pasta:         getenv("PASTA_LIVROS", "livros_cifrados"),
		SiteURL:       getenv("SITE_URL", "http://localhost:8080"),
		Segredo:       segredo,
		ChavePacote:   ed25519.PublicKey(pub),
		LoteMinimo:    lote,
		IntervaloLote: intervalo,
	})
	if err := c.AtualizarCatalogo(ctx); err != nil {
		log.Warn("catálogo ainda indisponível; tento de novo a cada entrega", "erro", err)
	}
	go c.RodarReportes(ctx, func(err error) { log.Error("reporte ao site", "erro", err) })

	srv := &http.Server{
		Addr:              getenv("ENDERECO", "localhost:8081"),
		Handler:           c.Rotas(log),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       120 * time.Second,
		WriteTimeout:      120 * time.Second,
	}
	erros := make(chan error, 1)
	go func() { erros <- srv.ListenAndServe() }()
	os.Stderr.WriteString("cdn no ar em http://" + srv.Addr + "\n")
	select {
	case err := <-erros:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	desligar, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelar()
	return srv.Shutdown(desligar)
}

func getenv(chave, padrao string) string {
	if v := os.Getenv(chave); v != "" {
		return v
	}
	return padrao
}
