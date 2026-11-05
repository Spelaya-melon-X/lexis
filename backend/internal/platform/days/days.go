// Package days — разбор календарных дат из запроса и общие правила для разделов,
// где данные привязаны к дню (дневник, привычки — SPEC §17).
//
// Отдельно от clock: clock — про время и не знает об HTTP, а здесь ошибки сразу
// *httpx.Error с кодами для фронта. Даты — в соглашении clock (00:00 UTC, см. clock.DateOf).
package days

import (
	"fmt"
	"time"

	"lexis/internal/platform/clock"
	"lexis/internal/platform/httpx"
)

// MaxRange — самое длинное окно ?from=&to= (год с хвостиком, как у stats/calendar).
const MaxRange = 366

// Parse — дата "YYYY-MM-DD" из URL/тела; 400 invalid_date.
func Parse(s string) (time.Time, error) {
	d, err := clock.ParseDate(s)
	if err != nil {
		return time.Time{}, httpx.BadRequest("invalid_date", "date must be YYYY-MM-DD")
	}
	return d, nil
}

// ParseRange — окно ?from=&to= включительно. По умолчанию to = today, from = to − (def − 1).
// Будущие дни в окне допустимы (календарь месяца целиком) — они просто пустые.
func ParseRange(fromS, toS string, today time.Time, def int) (from, to time.Time, err error) {
	to = today
	if toS != "" {
		if to, err = Parse(toS); err != nil {
			return
		}
	}
	from = to.AddDate(0, 0, -(def - 1))
	if fromS != "" {
		if from, err = Parse(fromS); err != nil {
			return
		}
	}
	if from.After(to) || clock.DaysBetween(from, to) >= MaxRange {
		err = httpx.BadRequest("invalid_range", fmt.Sprintf("from must be <= to, at most %d days", MaxRange))
	}
	return
}

// CheckWritable: записывать можно сегодня и прошлые дни, но не будущие (§17.2, Q11).
func CheckWritable(day, today time.Time) error {
	if day.After(today) {
		return httpx.BadRequest("future_day", "cannot write a day that has not come yet")
	}
	return nil
}

// WeekStart — понедельник недели, в которую входит дата (недели с понедельника, как в §5).
func WeekStart(d time.Time) time.Time {
	return d.AddDate(0, 0, -((int(d.Weekday()) + 6) % 7)) // Пн = 0 … Вс = 6
}

// ParseWeek — понедельник из URL; другой день недели → 400 not_monday.
func ParseWeek(s string) (time.Time, error) {
	d, err := Parse(s)
	if err != nil {
		return time.Time{}, err
	}
	if !WeekStart(d).Equal(d) {
		return time.Time{}, httpx.BadRequest("not_monday", "week must start on Monday")
	}
	return d, nil
}
