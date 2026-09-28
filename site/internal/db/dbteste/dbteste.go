// Package dbteste cria bancos isolados para testes de integração.
package dbteste

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"tcc/site/internal/db"
)

// URLPadrao aponta para o Postgres do docker-compose.yml.
const URLPadrao = "postgres://livraria:livraria_dev@localhost:5432/livraria?sslmode=disable"

// Novo devolve um pool isolado num schema novo, já migrado, e apaga o schema
// ao fim do teste. Usa TEST_DATABASE_URL ou, na falta, URLPadrao.
// Se o banco não estiver no ar, o teste é pulado (os testes sem banco rodam).
func Novo(t testing.TB) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = URLPadrao
	}

	sufixo := make([]byte, 6)
	rand.Read(sufixo)
	schema := "teste_" + hex.EncodeToString(sufixo)

	raiz, err := db.Conectar(ctx, url)
	if err != nil {
		t.Skipf("Postgres indisponível (%v); suba com `docker compose up -d`", err)
	}
	if _, err := raiz.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		raiz.Close()
		t.Fatalf("criando schema de teste: %v", err)
	}

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := db.ConectarComConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		pool.Close()
		raiz.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		raiz.Close()
	})

	if err := db.Migrar(ctx, pool); err != nil {
		t.Fatalf("migrando schema de teste: %v", err)
	}
	return pool
}
