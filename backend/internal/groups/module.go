// Package groups — список групп со статусом дня, аналитика группы, переименование/цвет/удаление.
package groups

import (
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lexis/internal/app"
	"lexis/internal/platform/clock"
	"lexis/internal/platform/httpx"
)

type Module struct {
	svc *Service
}

var _ app.Module = (*Module)(nil)

// New: wl — обычно words.NewService(...). Сервис words без состояния (пул, часы, шина),
// так что отдельный экземпляр для groups ничем не отличается от того, что в модуле words.
func New(pool *pgxpool.Pool, clk clock.Clock, wl WordLister) *Module {
	return &Module{svc: NewService(pool, clk, wl)}
}

func (m *Module) Name() string { return "groups" }

func (m *Module) Routes(r chi.Router) {
	r.Get("/", httpx.Handle(m.list))
	r.Get("/{id}", httpx.Handle(m.get))
	r.Patch("/{id}", httpx.Handle(m.update))
	r.Delete("/{id}", httpx.Handle(m.delete))
}
