package httpx

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

// Recover ловит панику в хендлере, логирует стек и отвечает 500 в нашем формате.
// Свой, а не chi/middleware.Recoverer: тот отвечает пустым телом, а фронту нужен {"error":...}.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			// ErrAbortHandler — штатный способ net/http оборвать ответ, его не глушим.
			if rec == http.ErrAbortHandler {
				panic(rec)
			}
			slog.ErrorContext(r.Context(), "panic",
				"method", r.Method, "path", r.URL.Path,
				"panic", rec, "stack", string(debug.Stack()))
			JSON(w, http.StatusInternalServerError,
				errorBody{Error: errorPayload{Code: "internal", Message: "internal server error"}})
		}()
		next.ServeHTTP(w, r)
	})
}

// RequestLog пишет одну строку slog на запрос: метод, путь, статус, длительность.
// time.Now() здесь ок: это замер латентности, а не бизнес-«сейчас» из clock.
func RequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		// Успешные GET (списки, статистика) — шум, уводим в Debug; мутации и ошибки — в Info.
		level := slog.LevelInfo
		if rec.status < 400 && r.Method == http.MethodGet {
			level = slog.LevelDebug
		}
		slog.Log(r.Context(), level, "http",
			"method", r.Method, "path", r.URL.Path,
			"status", rec.status, "dur", time.Since(start))
	})
}

// statusRecorder запоминает код ответа: стандартный ResponseWriter его не отдаёт.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wroteHeader {
		s.status = code
		s.wroteHeader = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	s.wroteHeader = true // неявный 200
	return s.ResponseWriter.Write(b)
}

// Unwrap нужен http.ResponseController (Flush, SetWriteDeadline) — он
// ищет исходный writer через эту цепочку.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }
