// Package stats — вкладка «Статистика»: сводка за день, heatmap, тренды (SPEC §8).
package stats

import (
	"net/http"

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

func New(pool *pgxpool.Pool, clk clock.Clock) *Module {
	return &Module{svc: NewService(pool, clk)}
}

func (m *Module) Name() string { return "stats" }

func (m *Module) Routes(r chi.Router) {
	r.Get("/overview", httpx.Handle(m.overview))
	r.Get("/calendar", httpx.Handle(m.calendar))
	r.Get("/trends", httpx.Handle(m.trends))
}

// overview — GET /api/stats/overview?day=YYYY-MM-DD (по умолчанию сегодня).
func (m *Module) overview(w http.ResponseWriter, r *http.Request) error {
	o, err := m.svc.Overview(r.Context(), r.URL.Query().Get("day"))
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, o)
	return nil
}

// calendar — GET /api/stats/calendar?from=&to= (YYYY-MM-DD, включительно; по умолчанию последний год).
func (m *Module) calendar(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	c, err := m.svc.Calendar(r.Context(), q.Get("from"), q.Get("to"))
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, c)
	return nil
}

// trends — GET /api/stats/trends?range=week|month (по умолчанию week).
func (m *Module) trends(w http.ResponseWriter, r *http.Request) error {
	rng := Range(r.URL.Query().Get("range"))
	if rng == "" {
		rng = RangeWeek
	}
	t, err := m.svc.Trends(r.Context(), rng)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, t)
	return nil
}
