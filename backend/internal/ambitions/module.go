package ambitions

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lexis/internal/platform/clock"
	"lexis/internal/platform/httpx"
)

type Module struct{ s *Service }

func New(pool *pgxpool.Pool, c clock.Clock) *Module { return &Module{NewService(pool, c)} }
func (m *Module) Name() string                      { return "ambitions" }
func (m *Module) Routes(r chi.Router) {
	r.Get("/", httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
		v, err := m.s.Get(r.Context())
		if err != nil {
			return err
		}
		httpx.JSON(w, 200, v)
		return nil
	}))
	for _, kind := range []string{"categories", "dreams", "projects", "steps", "goals", "years"} {
		r.Post("/"+kind, httpx.Handle(m.change(kind)))
		r.Put("/"+kind+"/{id}", httpx.Handle(m.change(kind)))
		r.Delete("/"+kind+"/{id}", httpx.Handle(m.change(kind)))
	}
	r.Post("/dreams/{id}/project", httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
		id, err := parseID(r)
		if err != nil {
			return err
		}
		var in Fields
		if err := httpx.Decode(w, r, &in); err != nil {
			return err
		}
		v, err := m.s.Convert(r.Context(), id, in)
		if err != nil {
			return err
		}
		httpx.JSON(w, 201, v)
		return nil
	}))
}
func parseID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, httpx.BadRequest("invalid_id", "invalid id")
	}
	return id, nil
}
func (m *Module) change(kind string) func(http.ResponseWriter, *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		var id int64
		var err error
		if r.Method != "POST" {
			id, err = parseID(r)
			if err != nil {
				return err
			}
		}
		var in Input
		if r.Method != "DELETE" {
			if err := httpx.Decode(w, r, &in); err != nil {
				return err
			}
		}
		v, err := m.s.Change(r.Context(), kind, id, r.Method, in)
		if err != nil {
			return err
		}
		status := 200
		if r.Method == "POST" {
			status = 201
		}
		httpx.JSON(w, status, v)
		return nil
	}
}
