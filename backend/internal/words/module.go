// Package words — ввод слов (парсер §4), сохранение пакетом и CRUD.
package words

import (
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lexis/internal/app"
	"lexis/internal/platform/clock"
	"lexis/internal/platform/eventbus"
	"lexis/internal/platform/httpx"
)

type Module struct {
	svc *Service
}

// Проверка на этапе компиляции, что Module реализует app.Module.
var _ app.Module = (*Module)(nil)

func New(pool *pgxpool.Pool, clk clock.Clock, bus eventbus.Publisher) *Module {
	return &Module{svc: NewService(pool, clk, bus)}
}

func (m *Module) Name() string { return "words" }

func (m *Module) Routes(r chi.Router) {
	r.Post("/parse", httpx.Handle(m.parse))
	r.Post("/bulk", httpx.Handle(m.bulk))
	r.Get("/", httpx.Handle(m.list))
	r.Patch("/{id}", httpx.Handle(m.update))
	r.Delete("/{id}", httpx.Handle(m.delete))
	r.Post("/{id}/unarchive", httpx.Handle(m.unarchive))
}
