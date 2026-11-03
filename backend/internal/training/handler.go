package training

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"lexis/internal/platform/httpx"
)

func (m *Module) start(w http.ResponseWriter, r *http.Request) error {
	var p StartParams
	if err := httpx.Decode(w, r, &p); err != nil {
		return err
	}
	res, err := m.svc.Start(r.Context(), p)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, res)
	return nil
}

func (m *Module) answer(w http.ResponseWriter, r *http.Request) error {
	id, err := sessionID(r)
	if err != nil {
		return err
	}
	var p AnswerParams
	if err := httpx.Decode(w, r, &p); err != nil {
		return err
	}
	res, err := m.svc.Answer(r.Context(), id, p)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, res)
	return nil
}

func (m *Module) finish(w http.ResponseWriter, r *http.Request) error {
	id, err := sessionID(r)
	if err != nil {
		return err
	}
	res, err := m.svc.Finish(r.Context(), id)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, res)
	return nil
}

func sessionID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, httpx.BadRequest("invalid_id", "session id must be a positive integer")
	}
	return id, nil
}
