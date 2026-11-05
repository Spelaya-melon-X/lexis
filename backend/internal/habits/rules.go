package habits

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"lexis/internal/platform/clock"
	"lexis/internal/platform/days"
	"lexis/internal/platform/httpx"
)

// GoalDates — проверенная цель: даты разобраны, заголовок нормализован.
type GoalDates struct {
	HabitID    *int64
	Title      string
	Start, End time.Time
}

// Normalize проверяет цель: даты YYYY-MM-DD, from <= to, не длиннее года;
// без привычки нужен заголовок (Q6).
func (g GoalInput) Normalize() (GoalDates, error) {
	title := strings.Join(strings.Fields(g.Title), " ")
	if utf8.RuneCountInString(title) > MaxGoalTitle {
		return GoalDates{}, httpx.BadRequest("text_too_long", fmt.Sprintf("title is longer than %d characters", MaxGoalTitle))
	}
	if g.HabitID == nil && title == "" {
		return GoalDates{}, httpx.BadRequest("title_required", "a goal without a habit needs a title")
	}
	start, err := days.Parse(g.From)
	if err != nil {
		return GoalDates{}, err
	}
	end, err := days.Parse(g.To)
	if err != nil {
		return GoalDates{}, err
	}
	if end.Before(start) || clock.DaysBetween(start, end) >= MaxGoalDays {
		return GoalDates{}, httpx.BadRequest("invalid_range", fmt.Sprintf("goal must be 1..%d days, from <= to", MaxGoalDays))
	}
	return GoalDates{HabitID: g.HabitID, Title: title, Start: start, End: end}, nil
}

// Progress — прогресс цели с привычкой на сегодня.
//
// Done для positive — дней с отметкой («бегал 4 из 9»), для negative — дней без срыва
// («не курил 4 из 9»): оба читаются как «сколько дней получилось». Elapsed — сколько дней
// цели уже прошло (включая сегодня), Total — длина цели. Цель — напоминание, провала нет.
type Progress struct {
	Done    int `json:"done"`
	Elapsed int `json:"elapsed"`
	Total   int `json:"total"`
}

// GoalProgress считает прогресс по дням с отметками checked ("YYYY-MM-DD"; отметки вне цели
// игнорируются). Ключ — строка, а не time.Time: == у time.Time сравнивает ещё и *Location.
func GoalProgress(kind string, start, end, today time.Time, checked map[string]bool) Progress {
	p := Progress{Total: clock.DaysBetween(start, end) + 1}
	last := end
	if today.Before(last) {
		last = today
	}
	if last.Before(start) {
		return p // цель ещё не началась
	}
	p.Elapsed = clock.DaysBetween(start, last) + 1
	marked := 0
	for d := start; !d.After(last); d = d.AddDate(0, 0, 1) {
		if checked[clock.FormatDate(d)] {
			marked++
		}
	}
	if kind == KindNegative {
		p.Done = p.Elapsed - marked
	} else {
		p.Done = marked
	}
	return p
}

// CheckOrder: новый порядок — перестановка всех существующих id (без лишних и дублей).
func CheckOrder(ids, existing []int64) error {
	bad := httpx.BadRequest("invalid_order", "ids must list every habit exactly once")
	if len(ids) != len(existing) {
		return bad
	}
	want := make(map[int64]bool, len(existing))
	for _, id := range existing {
		want[id] = true
	}
	for _, id := range ids {
		if !want[id] {
			return bad
		}
		delete(want, id) // второй раз тот же id уже не найдётся
	}
	return nil
}
