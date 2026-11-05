package sport

import (
	"fmt"
	"unicode/utf8"

	"lexis/internal/platform/httpx"
)

// Еда за день (§18.4, C2): список «что съел + примерные ккал». Чистые правила.

const (
	MaxMeals    = 50
	MaxMealLen  = 200
	MaxMealKcal = 10000
)

type Meal struct {
	Text string `json:"text"`
	Kcal *int   `json:"kcal"` // null — не знаю сколько
}

// NormalizeMeals: текст — trim и схлопнутые пробелы; пустые пункты выбрасываются (как пустые «Что классного»);
// не больше 50 пунктов, текст ≤ 200, ккал 0..10000 или null.
func NormalizeMeals(in []Meal) ([]Meal, error) {
	out := make([]Meal, 0, len(in))
	for _, m := range in {
		m.Text = collapse(m.Text)
		if m.Text == "" {
			continue
		}
		if utf8.RuneCountInString(m.Text) > MaxMealLen {
			return nil, httpx.BadRequest("text_too_long", fmt.Sprintf("a meal is longer than %d characters", MaxMealLen))
		}
		if m.Kcal != nil && (*m.Kcal < 0 || *m.Kcal > MaxMealKcal) {
			return nil, httpx.BadRequest("invalid_kcal", fmt.Sprintf("kcal must be 0..%d", MaxMealKcal))
		}
		out = append(out, m)
	}
	if len(out) > MaxMeals {
		return nil, httpx.BadRequest("too_many_items", fmt.Sprintf("a day has at most %d meals", MaxMeals))
	}
	return out, nil
}

// SumKcal — сумма известных ккал (пункты без ккал не считаются).
func SumKcal(meals []Meal) int {
	n := 0
	for _, m := range meals {
		if m.Kcal != nil {
			n += *m.Kcal
		}
	}
	return n
}
