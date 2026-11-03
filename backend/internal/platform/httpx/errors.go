package httpx

import (
	"errors"
	"log/slog"
	"net/http"
)

// Error — ошибка, которую можно показать клиенту. Всё, что не *Error,
// уходит наружу как 500 internal без деталей (детали — только в лог).
type Error struct {
	Status  int
	Code    string // машиночитаемый, фронт по нему выбирает текст из i18n
	Message string // человекочитаемый, для отладки
	Err     error  // причина, в ответ не попадает
}

func (e *Error) Error() string {
	if e.Err != nil {
		return e.Code + ": " + e.Message + ": " + e.Err.Error()
	}
	return e.Code + ": " + e.Message
}

func (e *Error) Unwrap() error { return e.Err }

func NewError(status int, code, msg string) *Error {
	return &Error{Status: status, Code: code, Message: msg}
}

func BadRequest(code, msg string) *Error { return NewError(http.StatusBadRequest, code, msg) }
func NotFound(msg string) *Error         { return NewError(http.StatusNotFound, "not_found", msg) }
func Conflict(code, msg string) *Error   { return NewError(http.StatusConflict, code, msg) }

type errorBody struct {
	Error errorPayload `json:"error"`
}

type errorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// WriteError пишет ошибку в едином формате {"error":{"code","message"}}.
//
// errors.As, а не type assertion: сервис может обернуть *Error через
// fmt.Errorf("op: %w", err), и мы всё равно его найдём.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	var he *Error
	if !errors.As(err, &he) {
		slog.ErrorContext(r.Context(), "unhandled error",
			"method", r.Method, "path", r.URL.Path, "err", err)
		he = NewError(http.StatusInternalServerError, "internal", "internal server error")
	} else if he.Status >= 500 {
		slog.ErrorContext(r.Context(), "server error",
			"method", r.Method, "path", r.URL.Path, "err", err)
	}
	JSON(w, he.Status, errorBody{Error: errorPayload{Code: he.Code, Message: he.Message}})
}

// HandlerFunc — хендлер, который возвращает ошибку вместо того, чтобы
// писать её сам. Убирает повторяющееся `if err != nil { WriteError; return }`.
type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

// Handle адаптирует HandlerFunc к http.HandlerFunc для chi.
func Handle(fn HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			WriteError(w, r, err)
		}
	}
}
