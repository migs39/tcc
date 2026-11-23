// Comando site: servidor web da livraria (RF01–RF08, RF13–RF16).
//
// Variáveis de ambiente:
//
//	DATABASE_URL  conexão com o PostgreSQL (padrão: Postgres do docker-compose.yml)
//	ENDERECO      endereço de escuta (padrão: localhost:8080)
//	CHAVE_PACOTE  arquivo com a chave Ed25519 do pacote do catálogo (padrão:
//	              chave_pacote.ed25519; criado se não existir)
//	OT_HELPER     caminho do binário ot-helper (padrão: ot-helper no PATH)
//	CDN_URL       endereço da CDN (padrão: http://localhost:8081)
//	SEGREDO_CDN   segredo compartilhado com a CDN (obrigatório para publicar
//	              livros e receber o reporte)
//	CAIXA_SAIDA   pasta onde os e-mails são gravados (padrão: caixa_saida),
//	              usada quando SMTP_HOST está vazio
//	SMTP_HOST, SMTP_PORTA (587), SMTP_USUARIO, SMTP_SENHA, SMTP_DE
//	              envio real de e-mail (D-014)
//	CLIENTE_URL, CLIENTE_SHA256  download do programa local (Release do
//	              repositório, fora deste site: D-004) e o hash mostrado na página
//	TARIFA_TOKEN_CENTAVOS  remuneração da CDN por token (padrão: 50, D-014)
package main

import (
	"context"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"tcc/site/internal/admin"
	"tcc/site/internal/catalogo"
	"tcc/site/internal/compra"
	"tcc/site/internal/db"
	"tcc/site/internal/pacote"
	"tcc/site/web"

	"tcc/protocolo"
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
	hAdmin := admin.NovosHandlers(svcAdmin, tpl, log)

	caminhoChave := getenv("CHAVE_PACOTE", "chave_pacote.ed25519")
	chave, criada, err := pacote.CarregarOuCriarChave(caminhoChave)
	if err != nil {
		return err
	}
	svcPacote := pacote.NovoServico(pool, chave)
	if criada {
		os.Stderr.WriteString("chave do pacote do catálogo criada em " + caminhoChave +
			"\nchave pública (embutir no programa local): " + hex.EncodeToString(svcPacote.ChavePublica()) + "\n")
	}

	segredoCDN := os.Getenv("SEGREDO_CDN")
	if segredoCDN == "" {
		os.Stderr.WriteString("aviso: SEGREDO_CDN vazio; publicação de livros e reporte da CDN ficam desativados\n")
	}
	var enviador compra.Enviador = compra.CaixaDeSaida{Pasta: getenv("CAIXA_SAIDA", "caixa_saida")}
	if h := os.Getenv("SMTP_HOST"); h != "" {
		enviador = compra.SMTP{Host: h, Porta: getenv("SMTP_PORTA", "587"), Usuario: os.Getenv("SMTP_USUARIO"),
			Senha: os.Getenv("SMTP_SENHA"), De: getenv("SMTP_DE", "livraria@localhost")}
		os.Stderr.WriteString("e-mail: SMTP em " + h + "\n")
	} else {
		os.Stderr.WriteString("e-mail: gravado em " + getenv("CAIXA_SAIDA", "caixa_saida") + " (SMTP_HOST vazio)\n")
	}
	fila := compra.NovaFila(enviador, log)
	svcCompra := compra.NovoServico(pool, protocolo.OTHelper{Caminho: os.Getenv("OT_HELPER")}, compra.GatewayMock{}, fila,
		&compra.ClienteCDN{URL: getenv("CDN_URL", "http://localhost:8081"), Segredo: segredoCDN})
	if v, err := strconv.Atoi(getenv("TARIFA_TOKEN_CENTAVOS", "50")); err == nil && v >= 0 {
		svcCompra.TarifaToken = v
	}
	hCompra := compra.NovosHandlers(svcCompra, hAdmin, segredoCDN, tpl, log)
	hPacote := pacote.NovosHandlers(svcPacote, tpl, log)
	hPacote.Cliente = pacote.NovoCliente(os.Getenv("CLIENTE_URL"), os.Getenv("CLIENTE_SHA256"))

	srv := &http.Server{
		Addr: getenv("ENDERECO", "localhost:8080"),
		Handler: NovoRoteador(tpl, log, hAdmin, catalogo.NovosHandlers(catalogo.NovoServico(pool), hAdmin, tpl, log),
			hPacote, hCompra),
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
