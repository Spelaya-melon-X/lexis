package tasks

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

type Module struct {
	svc *Service
}

var _ app.Module = (*Module)(nil)

func New(pool *pgxpool.Pool, clk clock.Clock, bus eventbus.Publisher) *Module {
	return &Module{svc: NewService(pool, clk, bus)}
}

func (m *Module) Name() string { return "tasks" }

func (m *Module) Routes(r chi.Router) {
	r.Get("/", httpx.Handle(m.list))
	r.Post("/", httpx.Handle(m.create))

	r.Get("/types", httpx.Handle(m.types))
	r.Post("/types", httpx.Handle(m.createType))
	r.Put("/types/order", httpx.Handle(m.reorderTypes))
	r.Patch("/types/{id}", httpx.Handle(m.updateType))
	r.Delete("/types/{id}", httpx.Handle(m.deleteType))

	r.Get("/columns", httpx.Handle(m.columns))
	r.Post("/columns", httpx.Handle(m.createColumn))
	r.Put("/columns/order", httpx.Handle(m.reorderColumns))
	r.Patch("/columns/{id}", httpx.Handle(m.renameColumn))
	r.Delete("/columns/{id}", httpx.Handle(m.deleteColumn))

	r.Get("/plan", httpx.Handle(m.calendar))
	r.Get("/plan/{day}", httpx.Handle(m.plan))
	r.Put("/plan/{day}", httpx.Handle(m.savePlan))
	r.Post("/plan/{day}/items", httpx.Handle(m.addPlanTask))
	r.Post("/plan/{day}/tasks", httpx.Handle(m.createPlanTask))
	r.Get("/settings", httpx.Handle(m.settings))
	r.Put("/settings", httpx.Handle(m.updateSettings))

	r.Post("/{id}/move", httpx.Handle(m.move))
	r.Post("/{id}/split", httpx.Handle(m.split))
	r.Get("/{id}/tree", httpx.Handle(m.tree))
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

// orderBody — тело PUT /…/order: полный список id в новом порядке.
type orderBody struct {
	IDs []int64 `json:"ids"`
}

// nameBody — тело POST/PATCH колонки.
type nameBody struct {
	Name string `json:"name"`
}

// ---------- дела ----------

// list — GET /api/tasks?done=false|true|all (по умолчанию false — невыполненные).
func (m *Module) list(w http.ResponseWriter, r *http.Request) error {
	f := Filter(r.URL.Query().Get("done"))
	if f == "" {
		f = FilterOpen
	}
	ts, err := m.svc.Tasks(r.Context(), f)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"tasks": ts})
	return nil
}

func (m *Module) create(w http.ResponseWriter, r *http.Request) error {
	var in TaskInput
	if err := httpx.Decode(w, r, &in); err != nil {
		return err
	}
	t, err := m.svc.Create(r.Context(), in)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, t)
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
	t, err := m.svc.Update(r.Context(), id, p)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, t)
	return nil
}

// move — POST /api/tasks/{id}/move {columnId, beforeId|null}.
func (m *Module) move(w http.ResponseWriter, r *http.Request) error {
	id, err := idParam(r)
	if err != nil {
		return err
	}
	var in MoveInput
	if err := httpx.Decode(w, r, &in); err != nil {
		return err
	}
	if in.ColumnID <= 0 {
		return httpx.BadRequest("invalid_column", "columnId must be a positive id")
	}
	if err := m.svc.Move(r.Context(), id, in); err != nil {
		return err
	}
	httpx.NoContent(w)
	return nil
}

// split — POST /api/tasks/{id}/split {steps: [...]} → {tasks: всё дерево}.
func (m *Module) split(w http.ResponseWriter, r *http.Request) error {
	id, err := idParam(r)
	if err != nil {
		return err
	}
	var in SplitInput
	if err := httpx.Decode(w, r, &in); err != nil {
		return err
	}
	ts, err := m.svc.Split(r.Context(), id, in)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"tasks": ts})
	return nil
}

// tree — GET /api/tasks/{id}/tree → {tasks: дерево от корня, обход в глубину}.
func (m *Module) tree(w http.ResponseWriter, r *http.Request) error {
	id, err := idParam(r)
	if err != nil {
		return err
	}
	ts, err := m.svc.Tree(r.Context(), id)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"tasks": ts})
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

// ---------- типы ----------

func (m *Module) types(w http.ResponseWriter, r *http.Request) error {
	ts, err := m.svc.Types(r.Context())
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"types": ts})
	return nil
}

func (m *Module) createType(w http.ResponseWriter, r *http.Request) error {
	var in TypeInput
	if err := httpx.Decode(w, r, &in); err != nil {
		return err
	}
	t, err := m.svc.CreateType(r.Context(), in)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, t)
	return nil
}

func (m *Module) updateType(w http.ResponseWriter, r *http.Request) error {
	id, err := idParam(r)
	if err != nil {
		return err
	}
	var p TypePatch
	if err := httpx.Decode(w, r, &p); err != nil {
		return err
	}
	t, err := m.svc.UpdateType(r.Context(), id, p)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, t)
	return nil
}

func (m *Module) deleteType(w http.ResponseWriter, r *http.Request) error {
	id, err := idParam(r)
	if err != nil {
		return err
	}
	if err := m.svc.DeleteType(r.Context(), id); err != nil {
		return err
	}
	httpx.NoContent(w)
	return nil
}

func (m *Module) reorderTypes(w http.ResponseWriter, r *http.Request) error {
	var body orderBody
	if err := httpx.Decode(w, r, &body); err != nil {
		return err
	}
	if err := m.svc.ReorderTypes(r.Context(), body.IDs); err != nil {
		return err
	}
	httpx.NoContent(w)
	return nil
}

// ---------- колонки ----------

func (m *Module) columns(w http.ResponseWriter, r *http.Request) error {
	cs, err := m.svc.Columns(r.Context())
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"columns": cs})
	return nil
}

func (m *Module) createColumn(w http.ResponseWriter, r *http.Request) error {
	var body nameBody
	if err := httpx.Decode(w, r, &body); err != nil {
		return err
	}
	c, err := m.svc.CreateColumn(r.Context(), body.Name)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, c)
	return nil
}

func (m *Module) renameColumn(w http.ResponseWriter, r *http.Request) error {
	id, err := idParam(r)
	if err != nil {
		return err
	}
	var body nameBody
	if err := httpx.Decode(w, r, &body); err != nil {
		return err
	}
	c, err := m.svc.RenameColumn(r.Context(), id, body.Name)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, c)
	return nil
}

// deleteColumn — DELETE /api/tasks/columns/{id}[?to=<id>]: to — куда перенести дела.
func (m *Module) deleteColumn(w http.ResponseWriter, r *http.Request) error {
	id, err := idParam(r)
	if err != nil {
		return err
	}
	var to *int64
	if s := r.URL.Query().Get("to"); s != "" {
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil || v <= 0 {
			return httpx.BadRequest("invalid_column", "to must be a positive column id")
		}
		to = &v
	}
	if err := m.svc.DeleteColumn(r.Context(), id, to); err != nil {
		return err
	}
	httpx.NoContent(w)
	return nil
}

func (m *Module) reorderColumns(w http.ResponseWriter, r *http.Request) error {
	var body orderBody
	if err := httpx.Decode(w, r, &body); err != nil {
		return err
	}
	if err := m.svc.ReorderColumns(r.Context(), body.IDs); err != nil {
		return err
	}
	httpx.NoContent(w)
	return nil
}

// ---------- распорядок дня ----------

func (m *Module) calendar(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	c, err := m.svc.Calendar(r.Context(), q.Get("from"), q.Get("to"))
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, c)
	return nil
}

// plan — GET /api/tasks/plan/{day} → Plan.
func (m *Module) plan(w http.ResponseWriter, r *http.Request) error {
	p, err := m.svc.Plan(r.Context(), chi.URLParam(r, "day"))
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, p)
	return nil
}

// savePlan — PUT /api/tasks/plan/{day} {items: [{taskId, start, blocks}]} → Plan.
func (m *Module) savePlan(w http.ResponseWriter, r *http.Request) error {
	var in PlanInput
	if err := httpx.Decode(w, r, &in); err != nil {
		return err
	}
	p, err := m.svc.SavePlan(r.Context(), chi.URLParam(r, "day"), in)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, p)
	return nil
}

func (m *Module) addPlanTask(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		TaskID int64 `json:"taskId"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		return err
	}
	p, err := m.svc.AddPlanTask(r.Context(), chi.URLParam(r, "day"), in.TaskID)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, p)
	return nil
}

func (m *Module) createPlanTask(w http.ResponseWriter, r *http.Request) error {
	var in TaskInput
	if err := httpx.Decode(w, r, &in); err != nil {
		return err
	}
	p, err := m.svc.CreatePlanTask(r.Context(), chi.URLParam(r, "day"), in)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, p)
	return nil
}

func (m *Module) settings(w http.ResponseWriter, r *http.Request) error {
	st, err := m.svc.Settings(r.Context())
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, st)
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
