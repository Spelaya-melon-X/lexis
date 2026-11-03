package groups

import (
	"time"

	"lexis/internal/platform/clock"
)

// Аналитика группы (SPEC §5). Чистые функции: «сейчас» и данные приходят параметрами.

// Levels — число слов на уровнях 1..5; индекс = уровень − 1.
type Levels [5]int

func (l Levels) Total() int {
	n := 0
	for _, c := range l {
		n += c
	}
	return n
}

// MasteredPercent — % слов на «Мастер» + «Архив», округление до целого. Пустая группа — 0.
func (l Levels) MasteredPercent() int {
	total := l.Total()
	if total == 0 {
		return 0
	}
	return ((l[3]+l[4])*100 + total/2) / total // +total/2 — округление половины вверх в целых
}

// RepeatedToday — статус дня (§5): была завершённая сессия сегодня ИЛИ нечего повторять.
func RepeatedToday(trainedToday bool, due int) bool {
	return trainedToday || due == 0
}

// WeekStart — понедельник 00:00 недели t (в TZ t). time.Weekday считает с воскресенья.
func WeekStart(t time.Time) time.Time {
	daysSinceMonday := (int(t.Weekday()) + 6) % 7
	return clock.StartOfDay(t).AddDate(0, 0, -daysSinceMonday)
}

// MonthStart — 1-е число месяца t, 00:00.
func MonthStart(t time.Time) time.Time {
	y, m, _ := t.Date()
	return time.Date(y, m, 1, 0, 0, 0, 0, t.Location())
}

// Range — окно графика продуктивности.
type Range string

const (
	RangeWeek  Range = "week"
	RangeMonth Range = "month"
)

// Days — сколько дней в окне, включая сегодня; ok=false для неизвестного значения.
func (r Range) Days() (int, bool) {
	switch r {
	case RangeWeek:
		return 7, true
	case RangeMonth:
		return 30, true
	}
	return 0, false
}

// SessionStat — завершённая сессия группы для графика.
type SessionStat struct {
	FinishedAt time.Time
	Total      int // слов с первым ответом
	Good       int
}

// DayPoint — точка графика. GoodShare nil — в этот день не было ответов (разрыв линии).
type DayPoint struct {
	Day       string   `json:"day"` // YYYY-MM-DD в TZ пользователя
	Sessions  int      `json:"sessions"`
	GoodShare *float64 `json:"goodShare"`
}

// Productivity раскладывает сессии по дням окна [today−days+1 .. today] и считает долю «Знаю»
// за день как Σgood / Σtotal (а не среднее долей: сессия на 2 слова не должна весить как на 50).
// Дни считаются в TZ now; сессии вне окна игнорируются.
func Productivity(sessions []SessionStat, now time.Time, days int) []DayPoint {
	first := clock.StartOfDay(now).AddDate(0, 0, -(days - 1))
	points := make([]DayPoint, days)
	totals := make([]int, days)
	goods := make([]int, days)
	for i := range points {
		points[i].Day = first.AddDate(0, 0, i).Format(time.DateOnly)
	}
	for _, s := range sessions {
		i := clock.DaysBetween(first, s.FinishedAt)
		if i < 0 || i >= days {
			continue
		}
		points[i].Sessions++
		totals[i] += s.Total
		goods[i] += s.Good
	}
	for i := range points {
		if totals[i] > 0 {
			share := float64(goods[i]) / float64(totals[i])
			points[i].GoodShare = &share
		}
	}
	return points
}
