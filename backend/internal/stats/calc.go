package stats

import (
	"slices"
	"time"

	"lexis/internal/platform/clock"
)

// Статистика (SPEC §8). Чистые функции: «сейчас» и строки из БД приходят параметрами,
// дни считаются в TZ now (время из pgx приходит в UTC — приводим через clock).

// Streak — серия дней подряд с хотя бы одной завершённой сессией (обычной или free).
type Streak struct {
	Current int `json:"current"`
	Best    int `json:"best"`
}

// Streaks считает серии по моментам завершения сессий (порядок и TZ — любые).
// Текущая серия не рвётся, пока идёт сегодняшний день: если сегодня ещё не тренировался,
// она заканчивается вчера. Пропущен вчерашний день — текущая серия 0.
func Streaks(finished []time.Time, now time.Time) Streak {
	loc := now.Location()
	seen := make(map[string]bool, len(finished))
	days := make([]time.Time, 0, len(finished))
	for _, t := range finished {
		d := clock.StartOfDay(t.In(loc))
		if key := d.Format(time.DateOnly); !seen[key] {
			seen[key] = true
			days = append(days, d)
		}
	}
	if len(days) == 0 {
		return Streak{}
	}
	slices.SortFunc(days, func(a, b time.Time) int { return a.Compare(b) })

	var s Streak
	run := 1
	for i := 1; i < len(days); i++ {
		if clock.DaysBetween(days[i-1], days[i]) == 1 {
			run++
		} else {
			s.Best = max(s.Best, run)
			run = 1
		}
	}
	s.Best = max(s.Best, run)

	// run сейчас — серия, заканчивающаяся последним активным днём.
	if gap := clock.DaysBetween(days[len(days)-1], now); gap == 0 || gap == 1 {
		s.Current = run
	}
	return s
}

// SessionRow — завершённая сессия; поля — как в training_sessions.
type SessionRow struct {
	FinishedAt  time.Time
	GroupID     *int64
	Total       int // слов с первым ответом
	Good        int
	Hard        int
	Again       int
	DurationSec int
}

// DayStats — агрегаты за день. Words — Σ первых ответов по сессиям дня
// (слово, разобранное в двух сессиях, считается дважды — как «разобрано карточек»).
type DayStats struct {
	Day       string   `json:"day"` // YYYY-MM-DD в TZ пользователя
	Sessions  int      `json:"sessions"`
	Words     int      `json:"words"`
	Good      int      `json:"good"`
	Hard      int      `json:"hard"`
	Again     int      `json:"again"`
	Seconds   int      `json:"seconds"`   // время тренировок
	GoodShare *float64 `json:"goodShare"` // good / words; null — ответов не было
}

// BucketByDay раскладывает сессии по дням [first, first+days) — каждый день окна есть
// в ответе, даже пустой (фронту не нужно достраивать heatmap). Вне окна — игнорируются.
func BucketByDay(rows []SessionRow, first time.Time, days int) []DayStats {
	first = clock.StartOfDay(first)
	out := make([]DayStats, days)
	for i := range out {
		out[i].Day = first.AddDate(0, 0, i).Format(time.DateOnly)
	}
	for _, r := range rows {
		i := clock.DaysBetween(first, r.FinishedAt)
		if i < 0 || i >= days {
			continue
		}
		d := &out[i]
		d.Sessions++
		d.Words += r.Total
		d.Good += r.Good
		d.Hard += r.Hard
		d.Again += r.Again
		d.Seconds += r.DurationSec
	}
	for i := range out {
		if out[i].Words > 0 {
			share := float64(out[i].Good) / float64(out[i].Words)
			out[i].GoodShare = &share
		}
	}
	return out
}

// GroupsTrained — сколько разных групп пройдено (сессии с удалённой группой не считаются).
func GroupsTrained(rows []SessionRow) int {
	seen := map[int64]bool{}
	for _, r := range rows {
		if r.GroupID != nil {
			seen[*r.GroupID] = true
		}
	}
	return len(seen)
}
