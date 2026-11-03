// Package training — тренировочные сессии: SRS (srs.go), очередь карточек (queue.go), API.
package training

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

var _ app.Module = (*Module)(nil)

func New(pool *pgxpool.Pool, clk clock.Clock, bus eventbus.Publisher) *Module {
	return &Module{svc: NewService(pool, clk, bus)}
}

func (m *Module) Name() string { return "training" }

func (m *Module) Routes(r chi.Router) {
	r.Post("/start", httpx.Handle(m.start))
	r.Post("/{id}/answer", httpx.Handle(m.answer))
	r.Post("/{id}/finish", httpx.Handle(m.finish))
}
