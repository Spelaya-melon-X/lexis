package journal

import (
	"time"

	"lexis/internal/platform/clock"
)

// SummaryRow — строка дня из БД (только дни, по которым что-то записано).
type SummaryRow struct {
	Day      time.Time
	Rating   *int
	Dopamine int
	Wins     int
	HasStory bool
}

// FillDays раскладывает строки по всем дням окна [from, to] — в ответе есть каждый день,
// пустые тоже (как stats.BucketByDay): фронту не нужно угадывать пропуски.
// rows должны быть отсортированы по дню и лежать внутри окна.
func FillDays(from, to time.Time, rows []SummaryRow) []DaySummary {
	n := clock.DaysBetween(from, to) + 1
	if n <= 0 {
		return []DaySummary{}
	}
	out := make([]DaySummary, n)
	for i := range out {
		out[i].Day = clock.FormatDate(from.AddDate(0, 0, i))
	}
	for _, r := range rows {
		i := clock.DaysBetween(from, r.Day)
		if i < 0 || i >= n {
			continue
		}
		out[i] = DaySummary{Day: out[i].Day, Rating: r.Rating, DopamineLevel: r.Dopamine, Wins: r.Wins, HasStory: r.HasStory}
	}
	return out
}
