package habits

import (
	"context"
	"fmt"
	"time"

	"lexis/internal/habits/store"
	"lexis/internal/platform/clock"
	"lexis/internal/platform/days"
)

// PeriodStat — выполнение привычки за период (§17.5): неделя в недельном анализе,
// неделя/месяц на странице статистики.
type PeriodStat struct {
	HabitID     int64   `json:"habitId"`
	Name        string  `json:"name"`
	Kind        string  `json:"kind"`
	Color       *string `json:"color"`
	Unit        string  `json:"unit"`
	TrackAmount bool    `json:"trackAmount"`
	TrackRating bool    `json:"trackRating"`
	Archived    bool    `json:"archived"`
	Counted
}

// Counted — посчитанные числа (чистая часть, см. Count).
type Counted struct {
	// Days — учитываемые дни: с max(from, startDay) по min(to, today). Будущее и дни до
	// создания привычки не считаются — иначе у новой привычки было бы «1 из 30».
	Days int `json:"days"`
	// Checked — дней с отметкой в окне до сегодня (positive — сделал, negative — сорвался).
	Checked   int      `json:"checked"`
	Amount    *float64 `json:"amount"`    // сумма количества; nil — нет ни одного значения
	AvgRating *float64 `json:"avgRating"` // средняя оценка; nil — оценок нет
	// BestClean — negative: самая длинная серия учитываемых дней без срыва в окне.
	BestClean int `json:"bestClean"`
}

// DayValue — отметка для подсчёта: день "YYYY-MM-DD" и значения.
type DayValue struct {
	Day    string
	Rating *int16
	Amount *float64
}

// Count считает выполнение за окно [from, to] на сегодня today. vals — отметки привычки
// (могут быть и вне окна — такие игнорируются).
func Count(kind string, startDay, from, to, today time.Time, vals []DayValue) Counted {
	last := to
	if today.Before(last) {
		last = today
	}
	first := from
	if startDay.After(first) {
		first = startDay
	}

	var c Counted
	checked := make(map[string]bool, len(vals))
	var sum, ratings float64
	var nAmount, nRating int
	fromS, lastS := clock.FormatDate(from), clock.FormatDate(last)
	for _, v := range vals {
		// Строки YYYY-MM-DD сравниваются как даты. Отметки раньше даты создания считаем:
		// их ставили задним числом осознанно.
		if v.Day < fromS || v.Day > lastS || checked[v.Day] {
			continue
		}
		checked[v.Day] = true
		c.Checked++
		if v.Amount != nil {
			sum += *v.Amount
			nAmount++
		}
		if v.Rating != nil {
			ratings += float64(*v.Rating)
			nRating++
		}
	}
	if nAmount > 0 {
		c.Amount = &sum
	}
	if nRating > 0 {
		avg := ratings / float64(nRating)
		c.AvgRating = &avg
	}

	if first.After(last) {
		return c // привычка создана после окна или окно целиком в будущем
	}
	c.Days = clock.DaysBetween(first, last) + 1
	if kind == KindNegative {
		run := 0
		for d := first; !d.After(last); d = d.AddDate(0, 0, 1) {
			if checked[clock.FormatDate(d)] {
				run = 0
				continue
			}
			run++
			c.BestClean = max(c.BestClean, run)
		}
	}
	return c
}

// PeriodStats — выполнение всех привычек за окно: активные + архивные с отметками в окне.
// Привычка, созданная после окна и без отметок в нём, не попадает.
func (s *Service) PeriodStats(ctx context.Context, from, to time.Time) ([]PeriodStat, error) {
	q := store.New(s.pool)
	hs, err := q.ListHabits(ctx)
	if err != nil {
		return nil, fmt.Errorf("habit stats: habits: %w", err)
	}
	cs, err := q.ListCheckins(ctx, store.ListCheckinsParams{FromDay: from, ToDay: to})
	if err != nil {
		return nil, fmt.Errorf("habit stats: checkins: %w", err)
	}
	byHabit := make(map[int64][]DayValue)
	for _, c := range cs {
		byHabit[c.HabitID] = append(byHabit[c.HabitID], DayValue{Day: clock.FormatDate(c.Day), Rating: c.Rating, Amount: c.Amount})
	}

	today := clock.TodayDate(s.clock)
	out := make([]PeriodStat, 0, len(hs))
	for _, h := range hs {
		vals := byHabit[h.ID]
		if h.ArchivedAt != nil && len(vals) == 0 {
			continue
		}
		start := clock.DateOf(h.CreatedAt.In(s.clock.Location()))
		c := Count(h.Kind, start, from, to, today, vals)
		if c.Days == 0 && c.Checked == 0 {
			continue
		}
		out = append(out, PeriodStat{
			HabitID: h.ID, Name: h.Name, Kind: h.Kind, Color: h.Color, Unit: h.Unit,
			TrackAmount: h.TrackAmount, TrackRating: h.TrackRating, Archived: h.ArchivedAt != nil, Counted: c,
		})
	}
	return out, nil
}

// Stats — GET /stats: окно ?from=&to= (по умолчанию последние 30 дней).
func (s *Service) Stats(ctx context.Context, fromS, toS string) (from, to time.Time, out []PeriodStat, err error) {
	from, to, err = days.ParseRange(fromS, toS, clock.TodayDate(s.clock), defaultRange)
	if err != nil {
		return
	}
	out, err = s.PeriodStats(ctx, from, to)
	return
}
