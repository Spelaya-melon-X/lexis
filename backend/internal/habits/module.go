package habits

import (
	"net/http"
	"strconv"

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

func (m *Module) Name() string { return "habits" }

func (m *Module) Routes(r chi.Router) {
	r.Get("/", httpx.Handle(m.list))
	r.Post("/", httpx.Handle(m.create))
	// Статические пути регистрируем до /{id}: chi и так предпочитает их, но так нагляднее.
	r.Put("/order", httpx.Handle(m.reorder))

	r.Get("/stats", httpx.Handle(m.stats))
	r.Get("/checkins", httpx.Handle(m.checkins))
	r.Put("/checkins/{day}", httpx.Handle(m.saveDay))

	r.Get("/goals", httpx.Handle(m.goals))
	r.Post("/goals", httpx.Handle(m.createGoal))
	r.Put("/goals/{id}", httpx.Handle(m.updateGoal))
	r.Delete("/goals/{id}", httpx.Handle(m.deleteGoal))

	r.Get("/notes", httpx.Handle(m.notes))
	r.Post("/notes", httpx.Handle(m.createNote))
	r.Put("/notes/{id}", httpx.Handle(m.updateNote))
	r.Delete("/notes/{id}", httpx.Handle(m.deleteNote))

	r.Patch("/{id}", httpx.Handle(m.update))
	r.Delete("/{id}", httpx.Handle(m.delete))
}

func idParam(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, httpx.BadRequest("invalid_id", "id must be a positive integer")
	}
	return id, nil
}

// ---------- привычки ----------

// list — GET /api/habits[?archived=true].
func (m *Module) list(w http.ResponseWriter, r *http.Request) error {
	hs, err := m.svc.List(r.Context(), r.URL.Query().Get("archived") == "true")
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"habits": hs})
	return nil
}

func (m *Module) create(w http.ResponseWriter, r *http.Request) error {
	var f Fields
	if err := httpx.Decode(w, r, &f); err != nil {
		return err
	}
	h, err := m.svc.Create(r.Context(), f)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, h)
	return nil
}

func (m *Module) update(w http.ResponseWriter, r *http.Request) error {
	id, err := idParam(r)
	if err != nil {
		return err
	}
	var p Patch
	if err := httpx.Decode(w, r, &p); err != nil {
		return err
	}
	h, err := m.svc.Update(r.Context(), id, p)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, h)
	return nil
}

func (m *Module) delete(w http.ResponseWriter, r *http.Request) error {
	id, err := idParam(r)
	if err != nil {
		return err
	}
	if err := m.svc.Delete(r.Context(), id); err != nil {
		return err
	}
	httpx.NoContent(w)
	return nil
}

// reorder — PUT /api/habits/order {ids: [...]} — все привычки в новом порядке.
func (m *Module) reorder(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		IDs []int64 `json:"ids"`
	}
	if err := httpx.Decode(w, r, &body); err != nil {
		return err
	}
	if err := m.svc.Reorder(r.Context(), body.IDs); err != nil {
		return err
	}
	httpx.NoContent(w)
	return nil
}

// ---------- отметки ----------

// checkins — GET /api/habits/checkins?from=&to= (по умолчанию последние 30 дней).
func (m *Module) checkins(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	from, to, cs, err := m.svc.Checkins(r.Context(), q.Get("from"), q.Get("to"))
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"from": clock.FormatDate(from), "to": clock.FormatDate(to), "checkins": cs})
	return nil
}

// saveDay — PUT /api/habits/checkins/{day} {checkins: [{habitId, done, trigger?, rating?, amount?}]}.
func (m *Module) saveDay(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Checkins []CheckinInput `json:"checkins"`
	}
	if err := httpx.Decode(w, r, &body); err != nil {
		return err
	}
	cs, err := m.svc.SaveDay(r.Context(), chi.URLParam(r, "day"), body.Checkins)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"checkins": cs})
	return nil
}

// stats — GET /api/habits/stats?from=&to= (по умолчанию последние 30 дней).
func (m *Module) stats(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	from, to, st, err := m.svc.Stats(r.Context(), q.Get("from"), q.Get("to"))
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"from": clock.FormatDate(from), "to": clock.FormatDate(to), "habits": st})
	return nil
}

// ---------- цели ----------

// goals — GET /api/habits/goals[?from=&to=] (без окна — все цели).
func (m *Module) goals(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	gs, err := m.svc.Goals(r.Context(), q.Get("from"), q.Get("to"))
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"goals": gs})
	return nil
}

func (m *Module) createGoal(w http.ResponseWriter, r *http.Request) error {
	var in GoalInput
	if err := httpx.Decode(w, r, &in); err != nil {
		return err
	}
	g, err := m.svc.CreateGoal(r.Context(), in)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, g)
	return nil
}

func (m *Module) updateGoal(w http.ResponseWriter, r *http.Request) error {
	id, err := idParam(r)
	if err != nil {
		return err
	}
	var in GoalInput
	if err := httpx.Decode(w, r, &in); err != nil {
		return err
	}
	g, err := m.svc.UpdateGoal(r.Context(), id, in)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, g)
	return nil
}

func (m *Module) deleteGoal(w http.ResponseWriter, r *http.Request) error {
	id, err := idParam(r)
	if err != nil {
		return err
	}
	if err := m.svc.DeleteGoal(r.Context(), id); err != nil {
		return err
	}
	httpx.NoContent(w)
	return nil
}

// ---------- наблюдения ----------

// notes — GET /api/habits/notes?habit=<id>|general&kind=&before=&limit=.
func (m *Module) notes(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	f := NoteFilter{Kind: q.Get("kind")}
	switch h := q.Get("habit"); h {
	case "":
	case "general":
		f.General = true
	default:
		id, err := strconv.ParseInt(h, 10, 64)
		if err != nil || id <= 0 {
			return httpx.BadRequest("invalid_habit", "habit must be an id or general")
		}
		f.HabitID = id
	}
	if v := q.Get("before"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			return httpx.BadRequest("invalid_cursor", "before must be a positive integer")
		}
		f.Before = n
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 || n > maxNotesPerReq {
			return httpx.BadRequest("invalid_limit", "limit must be 1..100")
		}
		f.Limit = n
	}
	ns, next, err := m.svc.Notes(r.Context(), f)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"notes": ns, "next": next})
	return nil
}

func (m *Module) createNote(w http.ResponseWriter, r *http.Request) error {
	var in NoteInput
	if err := httpx.Decode(w, r, &in); err != nil {
		return err
	}
	n, err := m.svc.CreateNote(r.Context(), in)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, n)
	return nil
}

func (m *Module) updateNote(w http.ResponseWriter, r *http.Request) error {
	id, err := idParam(r)
	if err != nil {
		return err
	}
	var in NoteInput
	if err := httpx.Decode(w, r, &in); err != nil {
		return err
	}
	n, err := m.svc.UpdateNote(r.Context(), id, in)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, n)
	return nil
}

func (m *Module) deleteNote(w http.ResponseWriter, r *http.Request) error {
	id, err := idParam(r)
	if err != nil {
		return err
	}
	if err := m.svc.DeleteNote(r.Context(), id); err != nil {
		return err
	}
	httpx.NoContent(w)
	return nil
}
