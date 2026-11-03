// Package clock — единственный источник «сейчас» и «сегодня» в приложении.
//
// Бизнес-код берёт время только из Clock, никогда из time.Now(): так в тестах
// можно подставить Fixed и проверить, например, переход через полночь.
package clock

import (
	"sync"
	"time"
)

// Clock возвращает текущее время в таймзоне пользователя.
type Clock interface {
	Now() time.Time
	Location() *time.Location
}

// Real — настоящие часы в заданной таймзоне.
type Real struct {
	loc *time.Location
}

func NewReal(loc *time.Location) Real { return Real{loc: loc} }

func (r Real) Now() time.Time           { return time.Now().In(r.loc) }
func (r Real) Location() *time.Location { return r.loc }

// Fixed — управляемые часы для тестов. Безопасны для конкурентного доступа,
// т.к. тесты гоняются с -race, а время могут читать фоновые горутины.
type Fixed struct {
	mu sync.Mutex
	t  time.Time
}

// NewFixed создаёт часы, стоящие на t. Таймзона берётся из t.
func NewFixed(t time.Time) *Fixed { return &Fixed{t: t} }

func (f *Fixed) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.t
}

func (f *Fixed) Location() *time.Location { return f.Now().Location() }

func (f *Fixed) Set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.t = t
}

func (f *Fixed) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.t = f.t.Add(d)
}

// StartOfDay — полночь дня t в таймзоне t.
//
// Через time.Date, а не t.Truncate(24*time.Hour): Truncate считает от нуля
// UTC и для Москвы вернёт 03:00, а не 00:00.
func StartOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// DayBounds — полуинтервал [начало дня, начало следующего дня) для t.
// Удобно передавать в SQL: `x >= $start AND x < $end`.
func DayBounds(t time.Time) (start, end time.Time) {
	start = StartOfDay(t)
	return start, start.AddDate(0, 0, 1)
}

// Today — начало сегодняшнего дня по часам c.
func Today(c Clock) time.Time { return StartOfDay(c.Now()) }

// DaysBetween — число календарных дней от a до b (b позже → положительное).
// Оба момента сначала приводятся к таймзоне a, потом сравниваются только даты:
// 23:59 и 00:01 следующего дня — это 1 день, хотя прошло 2 минуты.
func DaysBetween(a, b time.Time) int {
	b = b.In(a.Location())
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	// В UTC нет переходов на летнее время, поэтому каждые сутки ровно 24ч.
	da := time.Date(ay, am, ad, 0, 0, 0, 0, time.UTC)
	db := time.Date(by, bm, bd, 0, 0, 0, 0, time.UTC)
	return int(db.Sub(da).Hours() / 24)
}
