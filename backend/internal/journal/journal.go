// Package journal — раздел «День» (SPEC §17.2): оценка дня, «что классного сделал»,
// рассказ, дофаминовая шкала. Недельный анализ (§17.3) появится здесь же в фазе 4.
package journal

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"lexis/internal/platform/clock"
	"lexis/internal/platform/httpx"
)

const (
	DopamineSlots  = 7   // квадратиков на день
	MaxDopamineLen = 140 // символов в квадратике (то же ограничение — CHECK в схеме)
	MaxWinLen      = 200
	MaxWins        = 30
	MaxStoryLen    = 20000
	defaultRange   = 30 // GET /days без from — последние 30 дней
)

// Day — запись дня в API. Дня без записи в БД нет — отдаём пустой, а не 404:
// для интерфейса «ещё ничего не написал» и «день не существует» одно и то же.
type Day struct {
	Day           string    `json:"day"`
	Rating        *int      `json:"rating"`
	Story         string    `json:"story"`
	Wins          []string  `json:"wins"`
	Dopamine      []*string `json:"dopamine"` // ровно DopamineSlots, nil — пустой квадратик
	DopamineLevel int       `json:"dopamineLevel"`
}

func emptyDay(day time.Time) Day {
	return Day{Day: clock.FormatDate(day), Wins: []string{}, Dopamine: make([]*string, DopamineSlots)}
}

// DaySummary — день в календаре и на графиках.
type DaySummary struct {
	Day           string `json:"day"`
	Rating        *int   `json:"rating"`
	DopamineLevel int    `json:"dopamineLevel"`
	Wins          int    `json:"wins"`
	HasStory      bool   `json:"hasStory"`
}

// Patch — тело PATCH /api/journal/days/{day}. Нет поля — не менять.
type Patch struct {
	Rating httpx.Optional[int] `json:"rating"` // null — снять оценку
	Story  *string             `json:"story"`
	Wins   *[]string           `json:"wins"` // заменяет список целиком, [] — очистить
}

// Normalize проверяет патч и приводит его к виду для записи: пункты списка trim-ятся,
// пустые выкидываются (пустое поле ввода в UI — не пункт). Ошибка — *httpx.Error (400).
func (p Patch) Normalize() (Patch, error) {
	if p.Rating.Set && p.Rating.Value != nil {
		if r := *p.Rating.Value; r < 1 || r > 10 {
			return p, httpx.BadRequest("invalid_rating", "rating must be 1..10 or null")
		}
	}
	if p.Story != nil && utf8.RuneCountInString(*p.Story) > MaxStoryLen {
		return p, httpx.BadRequest("text_too_long", fmt.Sprintf("story is longer than %d characters", MaxStoryLen))
	}
	if p.Wins != nil {
		wins := make([]string, 0, len(*p.Wins))
		for _, w := range *p.Wins {
			w = strings.TrimSpace(w)
			if w == "" {
				continue
			}
			if utf8.RuneCountInString(w) > MaxWinLen {
				return p, httpx.BadRequest("text_too_long", fmt.Sprintf("item is longer than %d characters", MaxWinLen))
			}
			wins = append(wins, w)
		}
		if len(wins) > MaxWins {
			return p, httpx.BadRequest("too_many_items", fmt.Sprintf("at most %d items per day", MaxWins))
		}
		p.Wins = &wins
	}
	return p, nil
}

// NormalizeDopamine — текст квадратика: trim; "" значит «погасить квадратик».
func NormalizeDopamine(slot int, text string) (string, error) {
	if slot < 0 || slot >= DopamineSlots {
		return "", httpx.BadRequest("invalid_slot", fmt.Sprintf("slot must be 0..%d", DopamineSlots-1))
	}
	text = strings.TrimSpace(text)
	if utf8.RuneCountInString(text) > MaxDopamineLen {
		return "", httpx.BadRequest("text_too_long", fmt.Sprintf("text is longer than %d characters", MaxDopamineLen))
	}
	return text, nil
}
