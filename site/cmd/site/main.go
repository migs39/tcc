// Comando site: servidor web da livraria (RF01–RF08, RF13–RF16).
//
// Variáveis de ambiente:
//
//	DATABASE_URL  conexão com o PostgreSQL (padrão: Postgres do docker-compose.yml)
//	ENDERECO      endereço de escuta (padrão: localhost:8080)
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tcc/site/internal/admin"
	"tcc/site/internal/db"
	"tcc/site/web"
)

const urlPadrao = "postgres://livraria:livraria_dev@localhost:5432/livraria?sslmode=disable"

func main() {
	// Só erros vão para o log. Não há log de acesso: o caminho e os parâmetros
	// das requisições públicas revelariam o que o visitante olhou (L1).
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	if err := rodar(log); err != nil {
		log.Error("site encerrado com erro", "erro", err)
		os.Exit(1)
	}
}

func rodar(log *slog.Logger) error {
	ctx, parar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer parar()

	pool, err := db.Conectar(ctx, getenv("DATABASE_URL", urlPadrao))
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := db.Migrar(ctx, pool); err != nil {
		return err
	}

	tpl, err := web.Carregar()
	if err != nil {
		return err
	}
	svcAdmin, err := admin.NovoServico(pool)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              getenv("ENDERECO", "localhost:8080"),
		Handler:           NovoRoteador(tpl, admin.NovosHandlers(svcAdmin, tpl, log), log),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	erros := make(chan error, 1)
	go func() { erros <- srv.ListenAndServe() }()
	os.Stderr.WriteString("site no ar em http://" + srv.Addr + "\n")

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
