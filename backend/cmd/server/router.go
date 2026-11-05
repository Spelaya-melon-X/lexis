package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lexis/internal/achievements"
	"lexis/internal/app"
	"lexis/internal/groups"
	"lexis/internal/habits"
	"lexis/internal/importexport"
	"lexis/internal/journal"
	"lexis/internal/platform/clock"
	"lexis/internal/platform/eventbus"
	"lexis/internal/platform/httpx"
	"lexis/internal/settings"
	"lexis/internal/stats"
	"lexis/internal/training"
	"lexis/internal/words"
	"lexis/web"
)

// modules — регистрация модулей. Новый модуль = одна строка здесь (SPEC §3).
//
// words.Service без состояния (пул, часы, шина), поэтому groups и importexport получают
// свой экземпляр через узкие интерфейсы, а не тянут модуль words целиком.
func modules(pool *pgxpool.Pool, clk clock.Clock, bus *eventbus.Bus) []app.Module {
	wordsSvc := words.NewService(pool, clk, bus)
	habitsSvc := habits.NewService(pool, clk)
	return []app.Module{
		words.New(pool, clk, bus),
		groups.New(pool, clk, wordsSvc),
		training.New(pool, clk, bus),
		stats.New(pool, clk),
		achievements.New(pool, clk, bus), // подписывается на шину — до старта HTTP
		settings.New(pool),
		importexport.New(pool, clk, wordsSvc),
		journal.New(pool, clk, habitsSvc), // сводке недели нужны привычки
		habits.New(pool, clk),
	}
}

// newRouter собирает HTTP-обработчик: /healthz, /api/<module>/..., остальное — фронт.
func newRouter(ctx context.Context, pool *pgxpool.Pool, clk clock.Clock, bus *eventbus.Bus) (http.Handler, error) {
	r := chi.NewRouter()
	r.Use(httpx.Recover, httpx.RequestLog)

	r.Get("/healthz", httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
		pingCtx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if err := pool.Ping(pingCtx); err != nil {
			return &httpx.Error{Status: http.StatusServiceUnavailable, Code: "db_unavailable", Message: "database is unavailable", Err: err}
		}
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return nil
	}))

	var starters []app.Starter
	r.Route("/api", func(api chi.Router) {
		for _, m := range modules(pool, clk, bus) {
			api.Route("/"+m.Name(), m.Routes)
			if s, ok := m.(app.Starter); ok {
				starters = append(starters, s)
			}
		}
		// Неизвестный /api/... — JSON-ошибка в нашем формате, а не index.html фронта.
		api.NotFound(httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
			return httpx.NotFound("no such API endpoint")
		}))
		api.MethodNotAllowed(httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
			return httpx.NewError(http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		}))
	})

	for _, s := range starters {
		if err := s.Start(ctx); err != nil {
			return nil, fmt.Errorf("start module: %w", err)
		}
	}

	r.Handle("/*", web.Handler())
	return r, nil
}
