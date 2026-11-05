package sport

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

func (m *Module) Name() string { return "sport" }

func (m *Module) Routes(r chi.Router) {
	r.Get("/workouts", httpx.Handle(m.workouts))
	r.Post("/workouts", httpx.Handle(m.create))
	r.Get("/workouts/{id}", httpx.Handle(m.get))
	r.Put("/workouts/{id}", httpx.Handle(m.update))
	r.Delete("/workouts/{id}", httpx.Handle(m.delete))
	r.Get("/titles", httpx.Handle(m.titles))

	r.Get("/meals/suggest", httpx.Handle(m.mealSuggestions))
	r.Get("/meals/{day}", httpx.Handle(m.food))
	r.Put("/meals/{day}", httpx.Handle(m.saveFood))
	r.Get("/stats", httpx.Handle(m.stats))
	r.Get("/exercises/{id}/progress", httpx.Handle(m.progress))
	r.Get("/exercises", httpx.Handle(m.exercises))
	r.Patch("/exercises/{id}", httpx.Handle(m.renameExercise))
	r.Delete("/exercises/{id}", httpx.Handle(m.deleteExercise))
}

func idParam(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, httpx.BadRequest("invalid_id", "id must be a positive integer")
	}
	return id, nil
}

// workouts — GET /api/sport/workouts?from=&to= (по умолчанию — текущая неделя).
func (m *Module) workouts(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	from, to, ws, err := m.svc.Workouts(r.Context(), q.Get("from"), q.Get("to"))
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"from": clock.FormatDate(from), "to": clock.FormatDate(to), "workouts": ws})
	return nil
}

func (m *Module) get(w http.ResponseWriter, r *http.Request) error {
	id, err := idParam(r)
	if err != nil {
		return err
	}
	out, err := m.svc.Get(r.Context(), id)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, out)
	return nil
}

func (m *Module) create(w http.ResponseWriter, r *http.Request) error {
	var in WorkoutInput
	if err := httpx.Decode(w, r, &in); err != nil {
		return err
	}
	out, err := m.svc.Create(r.Context(), in)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, out)
	return nil
}

func (m *Module) update(w http.ResponseWriter, r *http.Request) error {
	id, err := idParam(r)
	if err != nil {
		return err
	}
	var in WorkoutInput
	if err := httpx.Decode(w, r, &in); err != nil {
		return err
	}
	out, err := m.svc.Update(r.Context(), id, in)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, out)
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

func (m *Module) titles(w http.ResponseWriter, r *http.Request) error {
	ts, err := m.svc.Titles(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"titles": ts})
	return nil
}

func (m *Module) exercises(w http.ResponseWriter, r *http.Request) error {
	es, err := m.svc.Exercises(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"exercises": es})
	return nil
}

// renameExercise — PATCH /api/sport/exercises/{id} {name}.
func (m *Module) renameExercise(w http.ResponseWriter, r *http.Request) error {
	id, err := idParam(r)
	if err != nil {
		return err
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := httpx.Decode(w, r, &body); err != nil {
		return err
	}
	e, err := m.svc.RenameExercise(r.Context(), id, body.Name)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, e)
	return nil
}

func (m *Module) deleteExercise(w http.ResponseWriter, r *http.Request) error {
	id, err := idParam(r)
	if err != nil {
		return err
	}
	if err := m.svc.DeleteExercise(r.Context(), id); err != nil {
		return err
	}
	httpx.NoContent(w)
	return nil
}

// ---------- еда, статистика, прогресс (C2) ----------

// food — GET /api/sport/meals/{day} → FoodDay.
func (m *Module) food(w http.ResponseWriter, r *http.Request) error {
	f, err := m.svc.Food(r.Context(), chi.URLParam(r, "day"))
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, f)
	return nil
}

// saveFood — PUT /api/sport/meals/{day} {meals: [{text, kcal}]} → FoodDay.
func (m *Module) saveFood(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Meals []Meal `json:"meals"`
	}
	if err := httpx.Decode(w, r, &body); err != nil {
		return err
	}
	f, err := m.svc.SaveFood(r.Context(), chi.URLParam(r, "day"), body.Meals)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, f)
	return nil
}

func (m *Module) mealSuggestions(w http.ResponseWriter, r *http.Request) error {
	ms, err := m.svc.MealSuggestions(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"meals": ms})
	return nil
}

// stats — GET /api/sport/stats?from=&to= (по умолчанию последние 7 дней).
func (m *Module) stats(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	st, err := m.svc.Stats(r.Context(), q.Get("from"), q.Get("to"))
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, st)
	return nil
}

// progress — GET /api/sport/exercises/{id}/progress?from=&to= → {exercise, points}.
func (m *Module) progress(w http.ResponseWriter, r *http.Request) error {
	id, err := idParam(r)
	if err != nil {
		return err
	}
	q := r.URL.Query()
	ex, points, err := m.svc.ExerciseProgress(r.Context(), id, q.Get("from"), q.Get("to"))
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"exercise": ex, "points": points})
	return nil
}
