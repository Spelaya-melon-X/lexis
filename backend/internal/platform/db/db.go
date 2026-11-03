// Package db — подключение к Postgres и прогон миграций при старте.
package db

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"lexis/migrations"
)

// Open создаёт пул соединений и проверяет, что БД отвечает.
//
// Ретраев нет: в compose app стартует только после healthcheck db
// (depends_on: service_healthy), а при локальном запуске быстрый фейл понятнее.
func Open(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("db open: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db ping: %w", err)
	}
	return pool, nil
}

// Migrate применяет все встроенные миграции, которых ещё нет в БД.
//
// Goose работает через database/sql, поэтому оборачиваем пул в *sql.DB
// (stdlib.OpenDBFromPool), а не открываем второе подключение по DSN.
// Используем goose.Provider, а не пакетные goose.Up/SetBaseFS:
// у Provider нет глобального состояния.
func Migrate(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) error {
	sqlDB := stdlib.OpenDBFromPool(pool)
	// Закрывает только обёртку, сам пул остаётся жив (см. доку OpenDBFromPool).
	defer sqlDB.Close()

	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS)
	if err != nil {
		return fmt.Errorf("migrate: new provider: %w", err)
	}

	results, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("migrate: up: %w", err)
	}
	for _, r := range results {
		log.Info("migration applied", "result", r.String())
	}
	return nil
}
