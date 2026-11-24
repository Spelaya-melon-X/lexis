package study

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lexis/internal/platform/clock"
	"lexis/internal/platform/httpx"
)

type Module struct{ s *Service }

func New(pool *pgxpool.Pool, c clock.Clock) *Module { return &Module{s: NewService(pool, c)} }
func (m *Module) Name() string                      { return "study" }
func (m *Module) Routes(r chi.Router) {
	r.Get("/knowledge", httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
		v, err := m.s.Knowledge(r.Context())
		if err != nil {
			return err
		}
		httpx.JSON(w, 200, v)
		return nil
	}))
	r.Post("/practice", httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
		var in PracticeInput
		if err := httpx.Decode(w, r, &in); err != nil {
			return err
		}
		v, err := m.s.Practice(r.Context(), in)
		if err != nil {
			return err
		}
		httpx.JSON(w, 200, v)
		return nil
	}))
	r.Post("/reviews", httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
		var in ReviewInput
		if err := httpx.Decode(w, r, &in); err != nil {
			return err
		}
		if err := m.s.Record(r.Context(), in); err != nil {
			return err
		}
		httpx.NoContent(w)
		return nil
	}))
	r.Get("/stats", httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
		days := 30
		if v := r.URL.Query().Get("days"); v != "" {
			var err error
			days, err = strconv.Atoi(v)
			if err != nil {
				return httpx.BadRequest("invalid_range", "invalid days")
			}
		}
		v, err := m.s.Stats(r.Context(), days)
		if err != nil {
			return err
		}
		httpx.JSON(w, 200, v)
		return nil
	}))
	r.Get("/", httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
		v, err := m.s.Get(r.Context())
		if err != nil {
			return err
		}
		httpx.JSON(w, 200, v)
		return nil
	}))
	r.Post("/parse", httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
		var in struct {
			Text    string `json:"text"`
			Grammar bool   `json:"grammar"`
		}
		if err := httpx.Decode(w, r, &in); err != nil {
			return err
		}
		httpx.JSON(w, 200, Parse(in.Text, in.Grammar))
		return nil
	}))
	for _, kind := range []string{"groups", "cards", "bulk"} {
		r.Post("/"+kind, httpx.Handle(m.change(kind)))
		if kind != "bulk" {
			r.Put("/"+kind+"/{id}", httpx.Handle(m.change(kind)))
			r.Delete("/"+kind+"/{id}", httpx.Handle(m.change(kind)))
		}
	}
}
func (m *Module) change(kind string) func(http.ResponseWriter, *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		var id int64
		if r.Method != "POST" {
			var err error
			id, err = strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
			if err != nil || id < 1 || id > 9007199254740991 {
				return httpx.BadRequest("invalid_id", "invalid id")
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
