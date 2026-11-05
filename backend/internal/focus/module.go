package focus

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lexis/internal/app"
	"lexis/internal/platform/clock"
	"lexis/internal/platform/eventbus"
	"lexis/internal/platform/httpx"
)

const (
	defaultLedger = 20
	maxLedger     = 100
)

type Module struct {
	svc *Service
}

var _ app.Module = (*Module)(nil)

// New: t — обычно tasks.NewService(...), d — journal.NewService(...): сервисы без состояния,
// отдельные экземпляры ничем не отличаются от тех, что в их модулях (как groups.WordLister).
func New(pool *pgxpool.Pool, clk clock.Clock, t Tasks, d Dopamine, bus eventbus.Publisher) *Module {
	return &Module{svc: NewService(pool, clk, t, d, bus)}
}

func (m *Module) Name() string { return "focus" }

func (m *Module) Routes(r chi.Router) {
	r.Get("/active", httpx.Handle(m.active))
	r.Post("/sessions", httpx.Handle(m.start))
	r.Get("/sessions/{id}", httpx.Handle(m.get))
	r.Post("/sessions/{id}/work", httpx.Handle(m.act(ActionWork)))
	r.Post("/sessions/{id}/break", httpx.Handle(m.act(ActionBreak)))
	r.Post("/sessions/{id}/resume", httpx.Handle(m.act(ActionResume)))
	r.Post("/sessions/{id}/distractions", httpx.Handle(m.distract))
	r.Post("/sessions/{id}/finish", httpx.Handle(m.finish))
	r.Get("/triggers", httpx.Handle(m.triggers))
	r.Get("/stats", httpx.Handle(m.stats))

	r.Get("/ritual", httpx.Handle(m.ritual))
	r.Put("/ritual", httpx.Handle(m.saveRitual))
	r.Get("/settings", httpx.Handle(m.settings))
	r.Put("/settings", httpx.Handle(m.updateSettings))

	r.Get("/rewards", httpx.Handle(m.rewards))
	r.Post("/rewards", httpx.Handle(m.createReward))
	r.Put("/rewards/order", httpx.Handle(m.reorderRewards))
	r.Patch("/rewards/{id}", httpx.Handle(m.updateReward))
	r.Delete("/rewards/{id}", httpx.Handle(m.deleteReward))
	r.Post("/rewards/{id}/buy", httpx.Handle(m.buy))
	r.Get("/points", httpx.Handle(m.points))
}

func idParam(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, httpx.BadRequest("invalid_id", "id must be a positive integer")
	}
	return id, nil
}

// ---------- сессии ----------

// active — GET /api/focus/active → {session: Session|null, now}. now — и когда сессии нет (для поправки часов).
func (m *Module) active(w http.ResponseWriter, r *http.Request) error {
	sess, err := m.svc.Active(r.Context())
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"session": sess, "now": m.svc.clock.Now()})
	return nil
}

func (m *Module) start(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		TaskID int64 `json:"taskId"`
	}
	if err := httpx.Decode(w, r, &body); err != nil {
		return err
	}
	sess, err := m.svc.Start(r.Context(), body.TaskID)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, sess)
	return nil
}

func (m *Module) get(w http.ResponseWriter, r *http.Request) error {
	id, err := idParam(r)
	if err != nil {
		return err
	}
	sess, err := m.svc.Get(r.Context(), id)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, sess)
	return nil
}

// act — POST /sessions/{id}/work|break|resume (без тела).
func (m *Module) act(a Action) func(http.ResponseWriter, *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		id, err := idParam(r)
		if err != nil {
			return err
		}
		sess, err := m.svc.Act(r.Context(), id, a)
		if err != nil {
			return err
		}
		httpx.JSON(w, http.StatusOK, sess)
		return nil
	}
}

func (m *Module) distract(w http.ResponseWriter, r *http.Request) error {
	id, err := idParam(r)
	if err != nil {
		return err
	}
	var in DistractionInput
	if err := httpx.Decode(w, r, &in); err != nil {
		return err
	}
	sess, err := m.svc.Distract(r.Context(), id, in)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, sess)
	return nil
}

func (m *Module) finish(w http.ResponseWriter, r *http.Request) error {
	id, err := idParam(r)
	if err != nil {
		return err
	}
	var body struct {
		Outcome Outcome `json:"outcome"`
	}
	if err := httpx.Decode(w, r, &body); err != nil {
		return err
	}
	if !body.Outcome.Valid() { // до транзакции; фазу проверит сервис
		return httpx.BadRequest("invalid_outcome", "outcome must be done, stopped or abandoned")
	}
	sess, err := m.svc.Finish(r.Context(), id, body.Outcome)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, sess)
	return nil
}

func (m *Module) triggers(w http.ResponseWriter, r *http.Request) error {
	ts, err := m.svc.Triggers(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"triggers": ts})
	return nil
}

// stats — GET /api/focus/stats?from=&to= (по умолчанию последние 7 дней).
func (m *Module) stats(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	st, err := m.svc.Stats(r.Context(), q.Get("from"), q.Get("to"))
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, st)
	return nil
}

// ---------- ритуал и настройки ----------

type ritualBody struct {
	Items []string `json:"items"`
}

func (m *Module) ritual(w http.ResponseWriter, r *http.Request) error {
	items, err := m.svc.Ritual(r.Context())
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, ritualBody{Items: items})
	return nil
}

func (m *Module) saveRitual(w http.ResponseWriter, r *http.Request) error {
	var body ritualBody
	if err := httpx.Decode(w, r, &body); err != nil {
		return err
	}
	items, err := m.svc.SaveRitual(r.Context(), body.Items)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, ritualBody{Items: items})
	return nil
}

func (m *Module) settings(w http.ResponseWriter, r *http.Request) error {
	httpx.JSON(w, http.StatusOK, m.svc.Settings(r.Context()))
	return nil
}

func (m *Module) updateSettings(w http.ResponseWriter, r *http.Request) error {
	var p SettingsPatch
	if err := httpx.Decode(w, r, &p); err != nil {
		return err
	}
	st, err := m.svc.UpdateSettings(r.Context(), p)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, st)
	return nil
}

// ---------- награды и очки ----------

// rewards — GET /api/focus/rewards[?archived=true].
func (m *Module) rewards(w http.ResponseWriter, r *http.Request) error {
	rs, err := m.svc.Rewards(r.Context(), r.URL.Query().Get("archived") == "true")
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"rewards": rs})
	return nil
}

func (m *Module) createReward(w http.ResponseWriter, r *http.Request) error {
	var in RewardInput
	if err := httpx.Decode(w, r, &in); err != nil {
		return err
	}
	rw, err := m.svc.CreateReward(r.Context(), in)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, rw)
	return nil
}

func (m *Module) updateReward(w http.ResponseWriter, r *http.Request) error {
	id, err := idParam(r)
	if err != nil {
		return err
	}
	var p RewardPatch
	if err := httpx.Decode(w, r, &p); err != nil {
		return err
	}
	rw, err := m.svc.UpdateReward(r.Context(), id, p)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, rw)
	return nil
}

func (m *Module) deleteReward(w http.ResponseWriter, r *http.Request) error {
	id, err := idParam(r)
	if err != nil {
		return err
	}
	if err := m.svc.DeleteReward(r.Context(), id); err != nil {
		return err
	}
	httpx.NoContent(w)
	return nil
}

func (m *Module) reorderRewards(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		IDs []int64 `json:"ids"`
	}
	if err := httpx.Decode(w, r, &body); err != nil {
		return err
	}
	if err := m.svc.ReorderRewards(r.Context(), body.IDs); err != nil {
		return err
	}
	httpx.NoContent(w)
	return nil
}

// buy — POST /api/focus/rewards/{id}/buy {dopamine?: bool} → {balance, dopamine}.
func (m *Module) buy(w http.ResponseWriter, r *http.Request) error {
	id, err := idParam(r)
	if err != nil {
		return err
	}
	var body struct {
		Dopamine bool `json:"dopamine"`
	}
	if err := httpx.Decode(w, r, &body); err != nil {
		return err
	}
	p, err := m.svc.Buy(r.Context(), id, body.Dopamine)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, p)
	return nil
}

// points — GET /api/focus/points?before=<id>&limit=1..100 → {balance, entries, next}.
func (m *Module) points(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	var before *int64
	if s := q.Get("before"); s != "" {
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil || v <= 0 {
			return httpx.BadRequest("invalid_cursor", "before must be a positive id")
		}
		before = &v
	}
	limit := defaultLedger
	if s := q.Get("limit"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v < 1 || v > maxLedger {
			return httpx.BadRequest("invalid_limit", "limit must be 1..100")
		}
		limit = v
	}
	balance, entries, next, err := m.svc.Points(r.Context(), before, limit)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"balance": balance, "entries": entries, "next": next})
	return nil
}
