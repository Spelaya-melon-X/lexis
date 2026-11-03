// Package settings — пользовательские настройки (SPEC §12): язык и лимит новых слов за сессию.
package settings

import (
	"encoding/json"
	"log/slog"

	"lexis/internal/platform/httpx"
	"lexis/internal/settings/store"
)

// Ключи в таблице settings. KeyNewWordsLimit читает и training (training.SettingNewWordsLimit) —
// совпадение проверяет тест.
const (
	KeyLanguage      = "language"
	KeyNewWordsLimit = "new_words_limit"
)

const MaxNewWordsLimit = 100

type Settings struct {
	Language      string `json:"language"`      // "ru" | "en"
	NewWordsLimit int    `json:"newWordsLimit"` // 0..MaxNewWordsLimit
}

func Defaults() Settings {
	return Settings{Language: "ru", NewWordsLimit: 10}
}

// Patch — тело PUT /api/settings: nil — не менять.
type Patch struct {
	Language      *string `json:"language"`
	NewWordsLimit *int    `json:"newWordsLimit"`
}

// Apply проверяет патч и применяет его к s. Ошибка — *httpx.Error (400).
func (p Patch) Apply(s Settings) (Settings, error) {
	if p.Language != nil {
		if *p.Language != "ru" && *p.Language != "en" {
			return s, httpx.BadRequest("invalid_language", "language must be ru or en")
		}
		s.Language = *p.Language
	}
	if p.NewWordsLimit != nil {
		if n := *p.NewWordsLimit; n < 0 || n > MaxNewWordsLimit {
			return s, httpx.BadRequest("invalid_new_words_limit", "newWordsLimit must be 0..100")
		}
		s.NewWordsLimit = *p.NewWordsLimit
	}
	return s, nil
}

// fromRows собирает Settings из строк таблицы поверх значений по умолчанию.
// Кривое значение (руками правили БД) не ломает чтение — остаётся дефолт, пишем в лог.
func fromRows(rows []store.Setting) Settings {
	s := Defaults()
	for _, r := range rows {
		var err error
		switch r.Key {
		case KeyLanguage:
			var v string
			if err = json.Unmarshal(r.Value, &v); err == nil && (v == "ru" || v == "en") {
				s.Language = v
			}
		case KeyNewWordsLimit:
			var v int
			if err = json.Unmarshal(r.Value, &v); err == nil && v >= 0 && v <= MaxNewWordsLimit {
				s.NewWordsLimit = v
			}
		default:
			continue // незнакомые ключи игнорируем (остались от старых версий)
		}
		if err != nil {
			slog.Warn("settings: bad value, using default", "key", r.Key, "value", string(r.Value), "err", err)
		}
	}
	return s
}

// rows — обратное преобразование для записи.
func (s Settings) rows() []store.UpsertSettingParams {
	lang, _ := json.Marshal(s.Language) // Marshal строки/числа не падает
	limit, _ := json.Marshal(s.NewWordsLimit)
	return []store.UpsertSettingParams{
		{Key: KeyLanguage, Value: lang},
		{Key: KeyNewWordsLimit, Value: limit},
	}
}
