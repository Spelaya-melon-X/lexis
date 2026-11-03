// Package achievements — достижения/бейджи (SPEC §8). Разблокируются по событиям eventbus.
package achievements

import (
	"net/http"

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

// New подписывает модуль на шину. Подписка — в конструкторе, а не в app.Starter.Start:
// Bus.Subscribe не блокирует и не требует ctx, а подписаться нужно до первых Publish,
// то есть до старта HTTP-сервера — main как раз создаёт модули раньше.
func New(pool *pgxpool.Pool, clk clock.Clock, bus eventbus.Subscriber) *Module {
	m := &Module{svc: NewService(pool, clk)}
	bus.Subscribe("achievements", m.svc.onEvent)
	return m
}

func (m *Module) Name() string { return "achievements" }

func (m *Module) Routes(r chi.Router) {
	r.Get("/", httpx.Handle(m.list))
}

func (m *Module) list(w http.ResponseWriter, r *http.Request) error {
	list, err := m.svc.List(r.Context())
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, map[string][]Achievement{"achievements": list})
	return nil
}
