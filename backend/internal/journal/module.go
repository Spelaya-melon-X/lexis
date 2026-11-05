package journal

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

// New: hs — обычно habits.NewService(...) (сервис без состояния, отдельный экземпляр — норма).
func New(pool *pgxpool.Pool, clk clock.Clock, hs HabitSource) *Module {
	return &Module{svc: NewService(pool, clk, hs)}
}

func (m *Module) Name() string { return "journal" }

func (m *Module) Routes(r chi.Router) {
	r.Get("/days", httpx.Handle(m.list))
	r.Get("/days/{day}", httpx.Handle(m.get))
	r.Patch("/days/{day}", httpx.Handle(m.patch))
	r.Put("/days/{day}/dopamine/{slot}", httpx.Handle(m.dopamine))
	r.Get("/weeks/{monday}", httpx.Handle(m.week))
	r.Put("/weeks/{monday}", httpx.Handle(m.saveWeek))
}

// list — GET /api/journal/days?from=&to= (YYYY-MM-DD включительно; по умолчанию 30 последних дней).
func (m *Module) list(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	from, to, days, err := m.svc.List(r.Context(), q.Get("from"), q.Get("to"))
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"from": clock.FormatDate(from), "to": clock.FormatDate(to), "days": days,
	})
	return nil
}

func (m *Module) get(w http.ResponseWriter, r *http.Request) error {
	d, err := m.svc.Get(r.Context(), chi.URLParam(r, "day"))
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, d)
	return nil
}

func (m *Module) patch(w http.ResponseWriter, r *http.Request) error {
	var p Patch
	if err := httpx.Decode(w, r, &p); err != nil {
		return err
	}
	d, err := m.svc.Update(r.Context(), chi.URLParam(r, "day"), p)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, d)
	return nil
}

// dopamine — PUT /api/journal/days/{day}/dopamine/{slot} {text}; пустой text гасит квадратик.
func (m *Module) dopamine(w http.ResponseWriter, r *http.Request) error {
	slot, err := strconv.Atoi(chi.URLParam(r, "slot"))
	if err != nil {
		return httpx.BadRequest("invalid_slot", "slot must be a number")
	}
	var body struct {
		Text string `json:"text"`
	}
	if err := httpx.Decode(w, r, &body); err != nil {
		return err
	}
	d, err := m.svc.SetDopamine(r.Context(), chi.URLParam(r, "day"), slot, body.Text)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, d)
	return nil
}

// week — GET /api/journal/weeks/{monday}: анализ недели + сводка.
func (m *Module) week(w http.ResponseWriter, r *http.Request) error {
	wk, err := m.svc.Week(r.Context(), chi.URLParam(r, "monday"))
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, wk)
	return nil
}

// saveWeek — PUT /api/journal/weeks/{monday} {blocks: [{topic, conclusion}]}; [] — удалить анализ.
func (m *Module) saveWeek(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Blocks []Block `json:"blocks"`
	}
	if err := httpx.Decode(w, r, &body); err != nil {
		return err
	}
	wk, err := m.svc.SaveWeek(r.Context(), chi.URLParam(r, "monday"), body.Blocks)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, wk)
	return nil
}
