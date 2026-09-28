// Package db abre a conexão com o PostgreSQL e aplica as migrações (D-003).
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"tcc/site/migrations"
)

// Conectar abre o pool e confirma que o banco responde.
func Conectar(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("configuração do banco inválida: %w", err)
	}
	return ConectarComConfig(ctx, cfg)
}

// ConectarComConfig é como Conectar, a partir de uma configuração já montada.
func ConectarComConfig(ctx context.Context, cfg *pgxpool.Config) (*pgxpool.Pool, error) {
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("abrindo pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("banco não responde: %w", err)
	}
	return pool, nil
}

// Migrar aplica todas as migrações pendentes.
func Migrar(ctx context.Context, pool *pgxpool.Pool) error {
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()

	provedor, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS)
	if err != nil {
		return fmt.Errorf("preparando migrações: %w", err)
	}
	if _, err := provedor.Up(ctx); err != nil {
		return fmt.Errorf("aplicando migrações: %w", err)
	}
	return nil
}
