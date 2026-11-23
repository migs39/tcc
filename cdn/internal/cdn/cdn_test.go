package cdn

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"tcc/protocolo"
)

func banco(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://livraria:livraria_dev@localhost:5432/livraria?sslmode=disable"
	}
	ctx := context.Background()
	raiz, err := pgxpool.New(ctx, url)
	if err != nil || raiz.Ping(ctx) != nil {
		t.Skip("Postgres indisponível")
	}
	schema := "teste_cdn_" + strings.ReplaceAll(strings.ToLower(t.Name()), "/", "_")
	raiz.Exec(ctx, "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
	if _, err := raiz.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	cfg, _ := pgxpool.ParseConfig(url)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close(); raiz.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); raiz.Close() })
	if err := Migrar(ctx, pool); err != nil {
		t.Fatal(err)
	}
	return pool
}

func token(t *testing.T, sk *rsa.PrivateKey) protocolo.Token {
	sem, _ := protocolo.NovaSemente()
	tp, err := protocolo.CegarTokens(&sk.PublicKey, sem.Fluxo("tokens"), 1)
	if err != nil {
		t.Fatal(err)
	}
	sig, _ := protocolo.AssinarCego(sk, tp.Cegados[0])
	toks, err := tp.Finalizar([][]byte{sig})
	if err != nil {
		t.Fatal(err)
	}
	return toks[0]
}

// RF17, D-011: entrega só com token válido da faixa do livro, uma vez.
// RF18: o reporte leva só os tokens e a quantidade, e marca como reportado.
func TestEntregaEReporte(t *testing.T) {
	ctx := context.Background()
	pool := banco(t)
	skA, _ := protocolo.GerarChaveToken()
	skB, _ := protocolo.GerarChaveToken()

	var recebido protocolo.ReporteCDN
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer s" {
			w.WriteHeader(401)
			return
		}
		jsonDecode(r, &recebido)
		w.Write([]byte(`{"aceitos":1}`))
	}))
	defer site.Close()

	c := Nova(pool, Config{Pasta: t.TempDir(), SiteURL: site.URL, Segredo: "s", LoteMinimo: 50})
	c.faixaLivro = map[int64]int64{1: 10, 2: 20}
	c.chaves = map[int64]*rsa.PublicKey{10: &skA.PublicKey, 20: &skB.PublicKey}
	if err := c.Guardar(1, []byte("cifrado")); err != nil {
		t.Fatal(err)
	}

	tA := token(t, skA)
	if _, err := c.Entregar(ctx, 1, token(t, skB).String()); err != ErrTokenInvalido {
		t.Fatalf("token de outra faixa aceito: %v", err)
	}
	if d, err := c.Entregar(ctx, 1, tA.String()); err != nil || string(d) != "cifrado" {
		t.Fatalf("entrega falhou: %v", err)
	}
	if _, err := c.Entregar(ctx, 1, tA.String()); err != ErrTokenUsado {
		t.Fatal("token reutilizado aceito")
	}
	if _, err := c.Entregar(ctx, 99, token(t, skA).String()); err != ErrLivroInexistente {
		t.Fatal("livro inexistente entregue")
	}

	// Abaixo do lote mínimo e dentro do intervalo: não envia.
	if n, _ := c.Reportar(ctx, false); n != 0 {
		t.Fatal("reportou antes do lote")
	}
	if n, err := c.Reportar(ctx, true); err != nil || n != 1 {
		t.Fatalf("reporte: %d, %v", n, err)
	}
	if recebido.Quantidade != 1 || len(recebido.Tokens) != 1 || recebido.Tokens[0] != tA.String() {
		t.Fatalf("lote inesperado: %+v", recebido)
	}
	var pendentes int
	pool.QueryRow(ctx, `SELECT count(*) FROM tokens_usados WHERE NOT reportado OR token IS NOT NULL`).Scan(&pendentes)
	if pendentes != 0 {
		t.Fatal("token continuou guardado depois de reportado")
	}
	// O token continua recusado depois de reportado.
	if _, err := c.Entregar(ctx, 1, tA.String()); err != ErrTokenUsado {
		t.Fatal("token reportado aceito de novo")
	}
	// A tabela não tem coluna de livro nem de horário (RF18).
	var cols string
	pool.QueryRow(ctx, `SELECT string_agg(column_name, ',' ORDER BY column_name) FROM information_schema.columns
		WHERE table_name = 'tokens_usados' AND table_schema = current_schema()`).Scan(&cols)
	if cols != "reportado,resumo,token" {
		t.Fatalf("colunas de tokens_usados: %s", cols)
	}
}

func jsonDecode(r *http.Request, v any) { json.NewDecoder(r.Body).Decode(v) }
