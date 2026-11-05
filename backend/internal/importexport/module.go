// Package importexport — импорт CSV/JSON, экспорт (CSV-список слов и полный JSON-бэкап),
// восстановление из бэкапа и очистка базы (SPEC §4, §12).
package importexport

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lexis/internal/app"
	"lexis/internal/platform/clock"
	"lexis/internal/platform/httpx"
)

// MaxImportBytes — лимит файла импорта. Бэкап года занятий — единицы МБ.
const MaxImportBytes = 20 << 20

type Module struct {
	svc *Service
}

var _ app.Module = (*Module)(nil)

func New(pool *pgxpool.Pool, clk clock.Clock, ws WordStore) *Module {
	return &Module{svc: NewService(pool, clk, ws)}
}

// Name — "data", а не "importexport": URL короче, и сюда же логично ложится reset.
func (m *Module) Name() string { return "data" }

func (m *Module) Routes(r chi.Router) {
	r.Post("/import", httpx.Handle(m.importFile))
	r.Get("/export", httpx.Handle(m.export))
	r.Post("/reset", httpx.Handle(m.reset))
}

// importFile — POST /api/data/import, multipart с полем "file". Формат определяется по содержимому.
func (m *Module) importFile(w http.ResponseWriter, r *http.Request) error {
	r.Body = http.MaxBytesReader(w, r.Body, MaxImportBytes)
	file, _, err := r.FormFile("file")
	if err != nil {
		var tooBig *http.MaxBytesError
		switch {
		case errors.As(err, &tooBig):
			return &httpx.Error{Status: http.StatusRequestEntityTooLarge, Code: "file_too_large", Message: "file is larger than 20 MB", Err: err}
		case errors.Is(err, http.ErrMissingFile):
			return httpx.BadRequest("missing_file", `multipart field "file" is required`)
		default:
			return &httpx.Error{Status: http.StatusBadRequest, Code: "invalid_multipart", Message: "expected multipart/form-data", Err: err}
		}
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("import: read file: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return httpx.BadRequest("empty_file", "file is empty")
	}
	res, err := m.svc.Import(r.Context(), data)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, res)
	return nil
}

// export — GET /api/data/export?format=csv|json (по умолчанию json — полный бэкап).
func (m *Module) export(w http.ResponseWriter, r *http.Request) error {
	day := m.svc.clock.Now().Format(time.DateOnly)
	switch r.URL.Query().Get("format") {
	case "", "json":
		b, err := m.svc.ExportBackup(r.Context())
		if err != nil {
			return err
		}
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="lexis-backup-%s.json"`, day))
		httpx.JSON(w, http.StatusOK, b)
	case "csv":
		// Сначала в буфер: если запись упадёт на середине, ещё можно ответить ошибкой, а не обрезанным файлом.
		var buf bytes.Buffer
		if err := m.svc.ExportCSV(r.Context(), &buf); err != nil {
			return err
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="lexis-words-%s.csv"`, day))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(buf.Bytes()) // заголовок ушёл — ошибку записи уже некому вернуть
	default:
		return httpx.BadRequest("invalid_format", "format must be csv or json")
	}
	return nil
}

// reset — POST /api/data/reset {"confirm": true, "scope"?: "english" | "all"}. Флаг — защита
// от случайного вызова (curl без тела, ошибка во фронте); подтверждение пользователя — на фронте.
// Без scope — только английский (как до v2: старые клиенты не сотрут дневник).
func (m *Module) reset(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Confirm bool   `json:"confirm"`
		Scope   string `json:"scope"`
	}
	if err := httpx.Decode(w, r, &req); err != nil {
		return err
	}
	if !req.Confirm {
		return httpx.BadRequest("confirm_required", `body must be {"confirm": true}`)
	}
	if req.Scope == "" {
		req.Scope = ScopeEnglish
	}
	if err := m.svc.Reset(r.Context(), req.Scope); err != nil {
		return err
	}
	httpx.NoContent(w)
	return nil
}
