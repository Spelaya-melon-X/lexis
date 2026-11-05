// Package habits — раздел «Привычки» (SPEC §17.4): привычки с опциональными полями,
// ежедневные отметки, цели на период, наблюдения и выводы.
//
// Чистые правила (валидация, прогресс цели) — в этом файле и в rules.go; БД — service.go.
package habits

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"lexis/internal/platform/httpx"
)

const (
	KindPositive = "positive"
	KindNegative = "negative"

	MaxNameLen    = 60
	MaxUnitLen    = 20
	MaxTriggerLen = 200
	MaxAmount     = 1e6
	MaxGoalTitle  = 200
	MaxGoalDays   = 366
	MaxNoteLen    = 20000

	NoteObservation = "observation"
	NoteInsight     = "insight"
)

// Habit — привычка в API.
type Habit struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	Kind         string  `json:"kind"`
	Color        *string `json:"color"`
	TrackTrigger bool    `json:"trackTrigger"`
	TrackRating  bool    `json:"trackRating"`
	TrackAmount  bool    `json:"trackAmount"`
	Unit         string  `json:"unit"`
	Archived     bool    `json:"archived"`
	// StartDay — дата создания в TZ пользователя: календарь не считает дни до неё «чистыми»
	// у негативной привычки.
	StartDay  string    `json:"startDay"`
	CreatedAt time.Time `json:"createdAt"`
}

// Fields — редактируемые поля привычки (тело POST; база для PATCH).
type Fields struct {
	Name         string  `json:"name"`
	Kind         string  `json:"kind"`
	Color        *string `json:"color"`
	TrackTrigger bool    `json:"trackTrigger"`
	TrackRating  bool    `json:"trackRating"`
	TrackAmount  bool    `json:"trackAmount"`
	Unit         string  `json:"unit"`
}

var colorRe = regexp.MustCompile(`^#[0-9a-f]{6}$`)

// Normalize: имя — trim и схлопнутые пробелы, цвет — lower-case #rrggbb, единица — trim.
func (f Fields) Normalize() (Fields, error) {
	f.Name = strings.Join(strings.Fields(f.Name), " ")
	if f.Name == "" || utf8.RuneCountInString(f.Name) > MaxNameLen {
		return f, httpx.BadRequest("invalid_habit_name", fmt.Sprintf("name must be 1..%d characters", MaxNameLen))
	}
	if f.Kind != KindPositive && f.Kind != KindNegative {
		return f, httpx.BadRequest("invalid_kind", "kind must be positive or negative")
	}
	if f.Color != nil {
		c := strings.ToLower(*f.Color)
		if !colorRe.MatchString(c) {
			return f, httpx.BadRequest("invalid_color", "color must be #rrggbb or null")
		}
		f.Color = &c
	}
	f.Unit = strings.TrimSpace(f.Unit)
	if utf8.RuneCountInString(f.Unit) > MaxUnitLen {
		return f, httpx.BadRequest("invalid_unit", fmt.Sprintf("unit must be at most %d characters", MaxUnitLen))
	}
	return f, nil
}

// Patch — тело PATCH /api/habits/{id}. Нет поля — не менять. Тип не меняется (см. queries).
type Patch struct {
	Name         *string                `json:"name"`
	Color        httpx.Optional[string] `json:"color"` // null — без цвета
	TrackTrigger *bool                  `json:"trackTrigger"`
	TrackRating  *bool                  `json:"trackRating"`
	TrackAmount  *bool                  `json:"trackAmount"`
	Unit         *string                `json:"unit"`
	Archived     *bool                  `json:"archived"`
}

// Apply накладывает патч на текущие поля и архивность. Архивирование ставит now,
// повторное «archived: true» дату не сдвигает.
func (p Patch) Apply(f Fields, archivedAt *time.Time, now time.Time) (Fields, *time.Time, error) {
	if p.Name != nil {
		f.Name = *p.Name
	}
	if p.Color.Set {
		f.Color = p.Color.Value
	}
	if p.TrackTrigger != nil {
		f.TrackTrigger = *p.TrackTrigger
	}
	if p.TrackRating != nil {
		f.TrackRating = *p.TrackRating
	}
	if p.TrackAmount != nil {
		f.TrackAmount = *p.TrackAmount
	}
	if p.Unit != nil {
		f.Unit = *p.Unit
	}
	if p.Archived != nil {
		switch {
		case !*p.Archived:
			archivedAt = nil
		case archivedAt == nil:
			archivedAt = &now
		}
	}
	f, err := f.Normalize()
	return f, archivedAt, err
}

// ---------- отметки ----------

// Checkin — отметка за день в API. Есть строка — привычка отмечена
// (positive — «сделал», negative — «сорвался», Q4).
type Checkin struct {
	HabitID int64    `json:"habitId"`
	Day     string   `json:"day"`
	Trigger *string  `json:"trigger"`
	Rating  *int     `json:"rating"`
	Amount  *float64 `json:"amount"`
}

// CheckinInput — одна привычка в форме дня. Done = false — снять отметку.
type CheckinInput struct {
	HabitID int64    `json:"habitId"`
	Done    bool     `json:"done"`
	Trigger *string  `json:"trigger"`
	Rating  *int     `json:"rating"`
	Amount  *float64 `json:"amount"`
}

// Tracks — какие поля включены у привычки.
type Tracks struct {
	Trigger, Rating, Amount bool
}

// ValidateCheckin проверяет отметку по включённым полям привычки (Q13):
// выключенное поле прислали → field_not_enabled; количество при trackAmount обязательно;
// оценка и триггер необязательны. Пустой триггер = нет триггера.
func ValidateCheckin(t Tracks, in CheckinInput) (CheckinInput, error) {
	if !in.Done {
		return CheckinInput{HabitID: in.HabitID}, nil // снимаем отметку: поля не важны
	}
	notEnabled := func(field string) error {
		return httpx.BadRequest("field_not_enabled", fmt.Sprintf("habit %d does not track %s", in.HabitID, field))
	}
	if in.Trigger != nil {
		if !t.Trigger {
			return in, notEnabled("trigger")
		}
		s := strings.TrimSpace(*in.Trigger)
		switch {
		case s == "":
			in.Trigger = nil
		case utf8.RuneCountInString(s) > MaxTriggerLen:
			return in, httpx.BadRequest("text_too_long", fmt.Sprintf("trigger is longer than %d characters", MaxTriggerLen))
		default:
			in.Trigger = &s
		}
	}
	if in.Rating != nil {
		if !t.Rating {
			return in, notEnabled("rating")
		}
		if r := *in.Rating; r < 1 || r > 10 {
			return in, httpx.BadRequest("invalid_rating", "rating must be 1..10")
		}
	}
	if in.Amount != nil {
		if !t.Amount {
			return in, notEnabled("amount")
		}
		// NaN/Inf из JSON не придут, но !(a >= 0) ловит и их — на случай вызова не из HTTP.
		if a := *in.Amount; !(a >= 0) || a > MaxAmount {
			return in, httpx.BadRequest("invalid_amount", "amount must be 0..1000000")
		}
	} else if t.Amount {
		return in, httpx.BadRequest("amount_required", fmt.Sprintf("habit %d needs an amount", in.HabitID))
	}
	return in, nil
}

// ---------- цели ----------

// Goal — цель на период. Progress — только у цели с привычкой.
type Goal struct {
	ID       int64     `json:"id"`
	HabitID  *int64    `json:"habitId"`
	Title    string    `json:"title"`
	From     string    `json:"from"`
	To       string    `json:"to"`
	Progress *Progress `json:"progress"`
}

// GoalInput — тело POST /goals и PUT /goals/{id} (цель заменяется целиком).
type GoalInput struct {
	HabitID *int64 `json:"habitId"`
	Title   string `json:"title"`
	From    string `json:"from"`
	To      string `json:"to"`
}

// NoteInput — тело POST /notes и PUT /notes/{id}.
type NoteInput struct {
	HabitID *int64 `json:"habitId"`
	Kind    string `json:"kind"`
	Body    string `json:"body"`
}

// Normalize: тело — trim по краям (переносы внутри сохраняются).
func (n NoteInput) Normalize() (NoteInput, error) {
	if n.Kind != NoteObservation && n.Kind != NoteInsight {
		return n, httpx.BadRequest("invalid_kind", "kind must be observation or insight")
	}
	n.Body = strings.TrimSpace(n.Body)
	if n.Body == "" {
		return n, httpx.BadRequest("empty_text", "note text is empty")
	}
	if utf8.RuneCountInString(n.Body) > MaxNoteLen {
		return n, httpx.BadRequest("text_too_long", fmt.Sprintf("note is longer than %d characters", MaxNoteLen))
	}
	return n, nil
}

// Note — запись ленты наблюдений.
type Note struct {
	ID        int64     `json:"id"`
	HabitID   *int64    `json:"habitId"`
	Kind      string    `json:"kind"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
