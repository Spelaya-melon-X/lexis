// Package httpx — общие HTTP-хелперы: JSON-ответы, единый формат ошибок, middleware.
package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
)

// MaxBodyBytes — лимит тела JSON-запроса. 1 МБ хватает на тысячи строк
// в /api/words/bulk; импорт файлов идёт multipart-ом со своим лимитом.
const MaxBodyBytes = 1 << 20

// JSON пишет v как JSON с указанным статусом.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	// Заголовок уже ушёл, поменять статус нельзя — остаётся только залогировать.
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("httpx: encode response", "err", err)
	}
}

// NoContent — 204 без тела.
func NoContent(w http.ResponseWriter) { w.WriteHeader(http.StatusNoContent) }

// Decode читает тело запроса в v. Строго: неизвестные поля и мусор после
// JSON-объекта — ошибка 400, чтобы опечатки во фронте всплывали сразу.
func Decode(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(v); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return &Error{Status: http.StatusRequestEntityTooLarge, Code: "body_too_large", Message: "request body too large", Err: err}
		}
		return &Error{Status: http.StatusBadRequest, Code: "invalid_json", Message: "invalid JSON body", Err: err}
	}
	// Второй Decode должен упереться в EOF, иначе в теле больше одного значения.
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return BadRequest("invalid_json", "body must contain a single JSON value")
	}
	return nil
}
