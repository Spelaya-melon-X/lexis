package settings

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lexis/internal/app"
	"lexis/internal/platform/httpx"
)

type Module struct {
	svc *Service
}

var _ app.Module = (*Module)(nil)

func New(pool *pgxpool.Pool) *Module {
	return &Module{svc: NewService(pool)}
}

func (m *Module) Name() string { return "settings" }

func (m *Module) Routes(r chi.Router) {
	r.Get("/", httpx.Handle(m.get))
	r.Put("/", httpx.Handle(m.put))
}

func (m *Module) get(w http.ResponseWriter, r *http.Request) error {
	s, err := m.svc.Get(r.Context())
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, s)
	return nil
}

func (m *Module) put(w http.ResponseWriter, r *http.Request) error {
	var p Patch
	if err := httpx.Decode(w, r, &p); err != nil {
		return err
	}
	s, err := m.svc.Update(r.Context(), p)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, s)
	return nil
}
