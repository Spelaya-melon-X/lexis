package clock

import "time"

// Календарные даты (колонки DATE: день дневника, отметка привычки — SPEC §17.7).
//
// Дата в Go — time.Time в 00:00 UTC. Почему UTC, а не полночь в TZ пользователя: дата — это
// «2026-11-03» без часового пояса. Полночь МСК — это 21:00 UTC предыдущего дня, и любое
// место, где значение приведут к UTC (pgx, JSON, сравнение), сдвинет дату на день.
// У 00:00 UTC такой ловушки нет: Date() даёт ту же дату, AddDate(0, 0, 1) — следующий день.
//
// Создавать даты только через DateOf / TodayDate / ParseDate.

// DateOf — календарная дата момента t в таймзоне t.
// Для «сегодня» передавать время из Clock (оно уже в TZ пользователя), см. TodayDate.
func DateOf(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// TodayDate — сегодняшняя дата в TZ пользователя.
func TodayDate(c Clock) time.Time { return DateOf(c.Now()) }

// ParseDate разбирает "YYYY-MM-DD" (несуществующие даты вроде 2026-02-30 — ошибка).
func ParseDate(s string) (time.Time, error) {
	return time.Parse(time.DateOnly, s) // без зоны в строке Parse возвращает UTC
}

// FormatDate — обратное к ParseDate.
func FormatDate(d time.Time) string { return d.Format(time.DateOnly) }
