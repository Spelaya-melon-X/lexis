package groups

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"lexis/internal/platform/httpx"
)

func (m *Module) list(w http.ResponseWriter, r *http.Request) error {
	groups, err := m.svc.List(r.Context())
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, map[string][]Group{"groups": groups})
	return nil
}

// get — GET /api/groups/:id?range=week|month (по умолчанию month).
func (m *Module) get(w http.ResponseWriter, r *http.Request) error {
	id, err := groupID(r)
	if err != nil {
		return err
	}
	rng := Range(r.URL.Query().Get("range"))
	if rng == "" {
		rng = RangeMonth
	}
	d, err := m.svc.Get(r.Context(), id, rng)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, d)
	return nil
}

func (m *Module) create(w http.ResponseWriter, r *http.Request) error {
	var p CreateParams
	if err := httpx.Decode(w, r, &p); err != nil {
		return err
	}
	g, err := m.svc.Create(r.Context(), p)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, g)
	return nil
}

func (m *Module) update(w http.ResponseWriter, r *http.Request) error {
	id, err := groupID(r)
	if err != nil {
		return err
	}
	var p UpdateParams
	if err := httpx.Decode(w, r, &p); err != nil {
		return err
	}
	g, err := m.svc.Update(r.Context(), id, p)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, g)
	return nil
}

// delete — DELETE /api/groups/:id?children=lift|delete (по умолчанию lift).
func (m *Module) delete(w http.ResponseWriter, r *http.Request) error {
	id, err := groupID(r)
	if err != nil {
		return err
	}
	children := Children(r.URL.Query().Get("children"))
	if children == "" {
		children = ChildrenLift
	}
	if err := m.svc.Delete(r.Context(), id, children); err != nil {
		return err
	}
	httpx.NoContent(w)
	return nil
}

func groupID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, httpx.BadRequest("invalid_id", "group id must be a positive integer")
	}
	return id, nil
}
