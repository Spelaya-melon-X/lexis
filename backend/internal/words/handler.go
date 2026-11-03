package words

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"lexis/internal/platform/httpx"
)

type textRequest struct {
	Text string `json:"text"`
}

// parse — только превью, в БД ничего не пишет (§4).
func (m *Module) parse(w http.ResponseWriter, r *http.Request) error {
	var req textRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, Parse(req.Text))
	return nil
}

func (m *Module) bulk(w http.ResponseWriter, r *http.Request) error {
	var req textRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		return err
	}
	res, err := m.svc.Bulk(r.Context(), req.Text)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, res)
	return nil
}

// list — GET /api/words?group=<id>&level=<1..5>, оба параметра необязательны.
func (m *Module) list(w http.ResponseWriter, r *http.Request) error {
	f, err := parseListFilter(r)
	if err != nil {
		return err
	}
	words, err := m.svc.List(r.Context(), f)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, map[string][]Word{"words": words})
	return nil
}

func parseListFilter(r *http.Request) (ListFilter, error) {
	var f ListFilter
	q := r.URL.Query()
	if s := q.Get("level"); s != "" {
		lvl, err := strconv.ParseInt(s, 10, 16)
		if err != nil || lvl < 1 || lvl > 5 {
			return f, httpx.BadRequest("invalid_level", "level must be 1..5")
		}
		l := int16(lvl)
		f.Level = &l
	}
	if s := q.Get("group"); s != "" {
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil || id <= 0 {
			return f, httpx.BadRequest("invalid_group", "group must be a positive id")
		}
		f.GroupID = &id
	}
	return f, nil
}

func (m *Module) update(w http.ResponseWriter, r *http.Request) error {
	id, err := wordID(r)
	if err != nil {
		return err
	}
	var p UpdateParams
	if err := httpx.Decode(w, r, &p); err != nil {
		return err
	}
	word, err := m.svc.Update(r.Context(), id, p)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, word)
	return nil
}

func (m *Module) delete(w http.ResponseWriter, r *http.Request) error {
	id, err := wordID(r)
	if err != nil {
		return err
	}
	if err := m.svc.Delete(r.Context(), id); err != nil {
		return err
	}
	httpx.NoContent(w)
	return nil
}

func (m *Module) unarchive(w http.ResponseWriter, r *http.Request) error {
	id, err := wordID(r)
	if err != nil {
		return err
	}
	word, err := m.svc.Unarchive(r.Context(), id)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, word)
	return nil
}

func wordID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, httpx.BadRequest("invalid_id", "word id must be a positive integer")
	}
	return id, nil
}
