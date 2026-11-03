//go:build integration

// Интеграционные тесты: нужен живой Postgres, обычный `go test` их не видит.
//
//	docker run --rm -d --name lexis-testdb -p 127.0.0.1:5444:5432 \
//	  -e POSTGRES_USER=lexis -e POSTGRES_PASSWORD=lexis -e POSTGRES_DB=lexis_test postgres:17-alpine
//	LEXIS_TEST_DSN=postgres://lexis:lexis@127.0.0.1:5444/lexis_test?sslmode=disable \
//	  go test -tags integration ./internal/platform/db/ -race
//
// Отдельная LEXIS_TEST_DSN, а не LEXIS_DB_DSN: тест откатывает схему до нуля,
// на рабочей базе это стёрло бы все слова.
package db

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"lexis/migrations"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("LEXIS_TEST_DSN")
	if dsn == "" {
		t.Skip("LEXIS_TEST_DSN not set")
	}
	pool, err := Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// resetSchema откатывает все миграции, чтобы тест начинался с пустой БД.
func resetSchema(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()
	p, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS)
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	if _, err := p.DownTo(context.Background(), 0); err != nil {
		t.Fatalf("down to 0: %v", err)
	}
}

func TestMigrate_UpDownUp(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	resetSchema(t, pool)
	if err := Migrate(ctx, pool, log); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	// Повторный запуск (рестарт контейнера) — no-op, не ошибка.
	if err := Migrate(ctx, pool, log); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}

	want := []string{
		"words", "word_examples", "groups", "word_groups", "training_sessions",
		"reviews", "achievements", "settings",
	}
	for _, table := range want {
		var exists bool
		err := pool.QueryRow(ctx, `SELECT to_regclass('public.' || $1) IS NOT NULL`, table).Scan(&exists)
		if err != nil {
			t.Fatalf("check %s: %v", table, err)
		}
		if !exists {
			t.Errorf("table %s missing after migrate", table)
		}
	}

	// Down-секция тоже рабочая: откат и повторный накат проходят чисто.
	resetSchema(t, pool)
	if err := Migrate(ctx, pool, log); err != nil {
		t.Fatalf("Migrate after down: %v", err)
	}
}

func TestSchema_Constraints(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	resetSchema(t, pool)
	if err := Migrate(ctx, pool, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	const insertWord = `INSERT INTO words (term, translation, level, next_review_at, created_at)
	                    VALUES ($1, $2, $3, now(), now())`
	if _, err := pool.Exec(ctx, insertWord, "Apple", "яблоко", 1); err != nil {
		t.Fatalf("insert word: %v", err)
	}

	tests := []struct {
		name string
		sql  string
		args []any
	}{
		{"duplicate case-insensitive", insertWord, []any{"apple", "ЯБЛОКО", 1}},
		{"level out of range", insertWord, []any{"pear", "груша", 6}},
		{"empty term", insertWord, []any{"", "пусто", 1}},
		{"bad strategy", `INSERT INTO training_sessions (strategy, started_at) VALUES ('xx', now())`, nil},
		{"forms not 3", `INSERT INTO words (term, forms, translation, next_review_at, created_at)
		                 VALUES ('go_went', '{go,went}', 'идти', now(), now())`, nil},
		{"empty sentence", `INSERT INTO word_examples (word_id, position, sentence)
		                    SELECT id, 0, '' FROM words LIMIT 1`, nil},
		{"duplicate achievement", `INSERT INTO achievements (code, unlocked_at)
		                           VALUES ('first_session', now()), ('first_session', now())`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, tt.sql, tt.args...); err == nil {
				t.Errorf("want constraint violation, got nil")
			}
		})
	}
}
