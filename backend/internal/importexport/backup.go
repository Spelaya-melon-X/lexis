package importexport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"lexis/internal/platform/clock"
	"lexis/internal/platform/httpx"
)

// BackupVersion — версия формата. Меняем при несовместимых изменениях; старые версии
// при необходимости конвертируем при восстановлении.
//
// v2 (§17.10) = v1 + необязательные секции journal и habits. Файлы v1 восстанавливаются
// как есть: новых секций в них нет, и эти разделы при восстановлении не трогаются.
const BackupVersion = 2

// Backup — полный снимок данных (§4: слова, группы, уровни, история).
// id внутри файла — только для ссылок между сущностями: при восстановлении
// записи получают новые id, ссылки переназначаются.
type Backup struct {
	Version      int                        `json:"version"`
	ExportedAt   time.Time                  `json:"exportedAt"`
	Settings     map[string]json.RawMessage `json:"settings"`
	Groups       []BackupGroup              `json:"groups"`
	Words        []BackupWord               `json:"words"`
	Sessions     []BackupSession            `json:"sessions"`
	Achievements []BackupAchievement        `json:"achievements"`
	// nil — секции нет в файле (бэкап v1): эти данные восстановление не трогает.
	Journal *BackupJournal `json:"journal,omitempty"`
	Habits  *BackupHabits  `json:"habits,omitempty"`
}

// BackupHabits — раздел «Привычки». id привычек — только для ссылок внутри файла.
type BackupHabits struct {
	Habits   []BackupHabit   `json:"habits"`
	Checkins []BackupCheckin `json:"checkins"`
	Goals    []BackupGoal    `json:"goals"`
	Notes    []BackupNote    `json:"notes"` // в порядке ленты (по возрастанию id)
}

type BackupHabit struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	Kind         string     `json:"kind"`
	Color        *string    `json:"color"`
	TrackTrigger bool       `json:"trackTrigger"`
	TrackRating  bool       `json:"trackRating"`
	TrackAmount  bool       `json:"trackAmount"`
	Unit         string     `json:"unit"`
	Position     int32      `json:"position"`
	ArchivedAt   *time.Time `json:"archivedAt"`
	CreatedAt    time.Time  `json:"createdAt"`
}

type BackupCheckin struct {
	HabitID   int64     `json:"habitId"`
	Day       string    `json:"day"`
	Trigger   *string   `json:"trigger"`
	Rating    *int16    `json:"rating"`
	Amount    *float64  `json:"amount"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type BackupGoal struct {
	HabitID   *int64    `json:"habitId"`
	Title     string    `json:"title"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	CreatedAt time.Time `json:"createdAt"`
}

type BackupNote struct {
	HabitID   *int64    `json:"habitId"`
	Kind      string    `json:"kind"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// BackupJournal — раздел «День».
type BackupJournal struct {
	Days  []BackupDay  `json:"days"`
	Weeks []BackupWeek `json:"weeks"` // нет в бэкапах до фазы 4 — тогда пусто
}

type BackupWeek struct {
	WeekStart string        `json:"weekStart"` // понедельник YYYY-MM-DD
	Blocks    []BackupBlock `json:"blocks"`
	CreatedAt time.Time     `json:"createdAt"`
	UpdatedAt time.Time     `json:"updatedAt"`
}

type BackupBlock struct {
	Topic      string `json:"topic"`
	Conclusion string `json:"conclusion"`
}

type BackupDay struct {
	Day       string           `json:"day"` // YYYY-MM-DD
	Rating    *int16           `json:"rating"`
	Story     string           `json:"story"`
	Wins      []string         `json:"wins"`
	Dopamine  []BackupDopamine `json:"dopamine"`
	CreatedAt time.Time        `json:"createdAt"`
	UpdatedAt time.Time        `json:"updatedAt"`
}

type BackupDopamine struct {
	Slot      int16     `json:"slot"`
	Text      string    `json:"text"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type BackupGroup struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Color     *string   `json:"color"`
	CreatedAt time.Time `json:"createdAt"`
}

type BackupWord struct {
	ID             int64      `json:"id"`
	Term           string     `json:"term"`
	Forms          []string   `json:"forms"`
	Translation    string     `json:"translation"`
	Level          int16      `json:"level"`
	Streak         int16      `json:"streak"`
	NextReviewAt   time.Time  `json:"nextReviewAt"`
	LastReviewedAt *time.Time `json:"lastReviewedAt"`
	LastGoodAt     *time.Time `json:"lastGoodAt"`
	CreatedAt      time.Time  `json:"createdAt"`
	Examples       []string   `json:"examples"` // разметка "I [go] home", в порядке position
	GroupIDs       []int64    `json:"groupIds"`
}

type BackupSession struct {
	ID          int64          `json:"id"`
	GroupID     *int64         `json:"groupId"`
	Strategy    string         `json:"strategy"`
	Free        bool           `json:"free"`
	StartedAt   time.Time      `json:"startedAt"`
	FinishedAt  *time.Time     `json:"finishedAt"`
	Total       int32          `json:"total"`
	Good        int32          `json:"good"`
	Hard        int32          `json:"hard"`
	Again       int32          `json:"again"`
	DurationSec int32          `json:"durationSec"`
	Reviews     []BackupReview `json:"reviews"`
}

type BackupReview struct {
	WordID         int64     `json:"wordId"`
	Answer         string    `json:"answer"`
	FirstInSession bool      `json:"firstInSession"`
	LevelBefore    int16     `json:"levelBefore"`
	LevelAfter     int16     `json:"levelAfter"`
	AnsweredAt     time.Time `json:"answeredAt"`
}

type BackupAchievement struct {
	Code       string    `json:"code"`
	UnlockedAt time.Time `json:"unlockedAt"`
}

// DecodeBackup разбирает файл бэкапа строго: неизвестное поле — скорее всего, не наш файл
// или другая версия, и восстанавливать «как получится» поверх всех данных нельзя.
func DecodeBackup(data []byte) (Backup, error) {
	var b Backup
	dec := json.NewDecoder(bytes.NewReader(bytes.TrimPrefix(data, utf8BOM)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&b); err != nil {
		return Backup{}, httpx.BadRequest("invalid_backup", "invalid backup: "+err.Error())
	}
	return b, validateBackup(b)
}

// validateBackup проверяет версию и ссылки до транзакции: висячая ссылка в файле —
// ошибка 400, а не 500 от внешнего ключа посреди восстановления.
// Значения полей (уровни 1..5, непустые строки, стратегии) проверяют CHECK-и схемы.
func validateBackup(b Backup) error {
	if b.Version < 1 || b.Version > BackupVersion {
		return httpx.BadRequest("unsupported_backup_version", fmt.Sprintf("backup version %d is not supported", b.Version))
	}
	bad := func(format string, args ...any) error {
		return httpx.BadRequest("invalid_backup", fmt.Sprintf(format, args...))
	}
	if b.Version == 1 && (b.Journal != nil || b.Habits != nil) {
		return bad("version 1 backup cannot contain journal or habits")
	}

	groups := make(map[int64]bool, len(b.Groups))
	for _, g := range b.Groups {
		if groups[g.ID] {
			return bad("duplicate group id %d", g.ID)
		}
		groups[g.ID] = true
	}
	wordIDs := make(map[int64]bool, len(b.Words))
	for _, w := range b.Words {
		if wordIDs[w.ID] {
			return bad("duplicate word id %d", w.ID)
		}
		wordIDs[w.ID] = true
		for _, gid := range w.GroupIDs {
			if !groups[gid] {
				return bad("word %d refers to unknown group %d", w.ID, gid)
			}
		}
	}
	sessions := make(map[int64]bool, len(b.Sessions))
	for _, s := range b.Sessions {
		if sessions[s.ID] {
			return bad("duplicate session id %d", s.ID)
		}
		sessions[s.ID] = true
		if s.GroupID != nil && !groups[*s.GroupID] {
			return bad("session %d refers to unknown group %d", s.ID, *s.GroupID)
		}
		for _, r := range s.Reviews {
			if !wordIDs[r.WordID] {
				return bad("review in session %d refers to unknown word %d", s.ID, r.WordID)
			}
		}
	}
	if b.Journal != nil {
		days := make(map[string]bool, len(b.Journal.Days))
		for _, d := range b.Journal.Days {
			// Дата — ключ (PK), а не просто значение: кривую ловим здесь, а не ошибкой COPY.
			if _, err := clock.ParseDate(d.Day); err != nil {
				return bad("invalid journal day %q", d.Day)
			}
			if days[d.Day] {
				return bad("duplicate journal day %s", d.Day)
			}
			days[d.Day] = true
		}
		weeks := make(map[string]bool, len(b.Journal.Weeks))
		for _, w := range b.Journal.Weeks {
			// Понедельник проверит и CHECK схемы, но кривая дата здесь — понятная 400, а не ошибка COPY.
			if _, err := clock.ParseDate(w.WeekStart); err != nil {
				return bad("invalid journal week %q", w.WeekStart)
			}
			if weeks[w.WeekStart] {
				return bad("duplicate journal week %s", w.WeekStart)
			}
			weeks[w.WeekStart] = true
		}
	}
	if b.Habits != nil {
		return validateHabits(*b.Habits, bad)
	}
	return nil
}

// validateHabits — ссылки и даты секции habits (значения полей проверяют CHECK-и схемы).
func validateHabits(h BackupHabits, bad func(string, ...any) error) error {
	ids := make(map[int64]bool, len(h.Habits))
	for _, x := range h.Habits {
		if ids[x.ID] {
			return bad("duplicate habit id %d", x.ID)
		}
		ids[x.ID] = true
	}
	ref := func(what string, id *int64) error {
		if id != nil && !ids[*id] {
			return bad("%s refers to unknown habit %d", what, *id)
		}
		return nil
	}
	date := func(what, s string) error {
		if _, err := clock.ParseDate(s); err != nil {
			return bad("invalid %s date %q", what, s)
		}
		return nil
	}
	seen := make(map[string]bool, len(h.Checkins))
	for _, c := range h.Checkins {
		if err := ref("checkin", &c.HabitID); err != nil {
			return err
		}
		if err := date("checkin", c.Day); err != nil {
			return err
		}
		key := fmt.Sprintf("%d/%s", c.HabitID, c.Day)
		if seen[key] {
			return bad("duplicate checkin %s", key)
		}
		seen[key] = true
	}
	for _, g := range h.Goals {
		if err := ref("goal", g.HabitID); err != nil {
			return err
		}
		if err := date("goal", g.From); err != nil {
			return err
		}
		if err := date("goal", g.To); err != nil {
			return err
		}
	}
	for _, n := range h.Notes {
		if err := ref("note", n.HabitID); err != nil {
			return err
		}
	}
	return nil
}
