// Package dbtest — живая тестовая БД для интеграционных тестов модулей.
//
// Отдельный пакет, а не хелпер в db_integration_test.go: неэкспортируемые функции
// из _test.go другим пакетам не видны, а повторять сброс схемы в каждом модуле не хочется.
// Тесты самого пакета db его не используют (dbtest импортирует db → был бы цикл).
package dbtest

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"lexis/internal/platform/db"
	"lexis/migrations"
)

// New открывает пул на LEXIS_TEST_DSN, откатывает схему до нуля и накатывает заново,
// так что каждый тест стартует с пустой БД. Без LEXIS_TEST_DSN тест пропускается.
//
// Все пакеты делят одну БД, поэтому интеграционные тесты гоняем с `-p 1` (SPEC §15).
func New(t testing.TB) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("LEXIS_TEST_DSN")
	if dsn == "" {
		t.Skip("LEXIS_TEST_DSN not set")
	}
	ctx := context.Background()

	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("dbtest: open: %v", err)
	}
	t.Cleanup(pool.Close)

	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()
	p, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS)
	if err != nil {
		t.Fatalf("dbtest: new provider: %v", err)
	}
	if _, err := p.DownTo(ctx, 0); err != nil {
		t.Fatalf("dbtest: down to 0: %v", err)
	}
	if err := db.Migrate(ctx, pool, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatalf("dbtest: migrate: %v", err)
	}
	return pool
}
