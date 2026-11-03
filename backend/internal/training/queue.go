package training

import (
	"math/rand/v2"
	"slices"
	"strings"
	"time"

	"lexis/internal/platform/clock"
	"lexis/internal/words"
)

// Очередь карточек сессии (SPEC §7). Чистые функции; случайность — параметром,
// чтобы тесты с фиксированным seed были детерминированы.

type Strategy string

const (
	StrategyEnRu    Strategy = "en_ru"
	StrategyRuEn    Strategy = "ru_en"
	StrategyContext Strategy = "context"
)

func (s Strategy) Valid() bool {
	return s == StrategyEnRu || s == StrategyRuEn || s == StrategyContext
}

// DefaultNewLimit — лимит новых слов за сессию, если в settings ничего нет.
const DefaultNewLimit = 10

// QueueWord — слово-кандидат в сессию.
type QueueWord struct {
	ID           int64
	Term         string
	Forms        []string
	Translation  string
	Level        int16
	NextReviewAt time.Time
	Examples     []words.Example
}

// Card — одна карточка очереди. Для context — с примером (подкарточка),
// у одного слова-глагола их может быть несколько.
type Card struct {
	WordID      int64          `json:"wordId"`
	Term        string         `json:"term"`
	Forms       []string       `json:"forms,omitempty"`
	Translation string         `json:"translation"`
	Level       int16          `json:"level"`
	Example     *words.Example `json:"example,omitempty"`
}

type BuildOptions struct {
	Strategy Strategy
	Now      time.Time
	NewLimit int  // только для обычной сессии
	Free     bool // все слова, один shuffle, без корзин и лимита
	Rand     *rand.Rand
}

// Build собирает очередь сессии.
//
// Обычная: корзины просроченные → сегодняшние → новые (не больше NewLimit), shuffle внутри
// корзины; архив и слова «на будущее» не попадают. Free: все слова, общий shuffle.
// В обоих случаях похожие карточки (одно слово, одинаковый перевод) разносятся.
func Build(ws []QueueWord, o BuildOptions) []Card {
	var buckets [][]QueueWord
	if o.Free {
		all := slices.Clone(ws) // чистая функция: срез вызывающего не перемешиваем
		shuffle(all, o.Rand)
		buckets = [][]QueueWord{all}
	} else {
		buckets = splitBuckets(ws, o.Now, o.NewLimit, o.Rand)
	}

	cardBuckets := make([][]Card, len(buckets))
	for b, bucket := range buckets {
		for _, w := range bucket {
			cardBuckets[b] = append(cardBuckets[b], expand(w, o.Strategy, o.Rand)...)
		}
	}
	return spreadSimilar(cardBuckets)
}

func splitBuckets(ws []QueueWord, now time.Time, newLimit int, r *rand.Rand) [][]QueueWord {
	dayStart, dayEnd := clock.DayBounds(now)
	var overdue, today, fresh []QueueWord
	for _, w := range ws {
		switch {
		case w.Level >= LevelArchive:
		case w.Level == LevelNew:
			fresh = append(fresh, w)
		case w.NextReviewAt.Before(dayStart):
			overdue = append(overdue, w)
		case w.NextReviewAt.Before(dayEnd):
			today = append(today, w)
		}
	}
	shuffle(overdue, r)
	shuffle(today, r)
	// Лимит — после shuffle, иначе в сессию всегда попадали бы одни и те же новые слова.
	shuffle(fresh, r)
	if len(fresh) > newLimit {
		fresh = fresh[:max(newLimit, 0)]
	}
	return [][]QueueWord{overdue, today, fresh}
}

// expand превращает слово в карточки по стратегии. context: у глагола с 3 формами —
// подкарточка на каждый пример, у обычного слова — один случайный пример;
// без примеров — карточка без примера (фронт покажет её как en_ru).
func expand(w QueueWord, s Strategy, r *rand.Rand) []Card {
	base := Card{WordID: w.ID, Term: w.Term, Forms: w.Forms, Translation: w.Translation, Level: w.Level}
	if s != StrategyContext || len(w.Examples) == 0 {
		return []Card{base}
	}
	if len(w.Forms) == 0 {
		ex := w.Examples[r.IntN(len(w.Examples))]
		base.Example = &ex
		return []Card{base}
	}
	cards := make([]Card, len(w.Examples))
	for i := range w.Examples {
		cards[i] = base
		cards[i].Example = &w.Examples[i]
	}
	return cards
}

// spreadSimilar склеивает корзины по порядку так, чтобы похожие карточки не шли подряд.
//
// Похожие = одинаковый перевод без учёта регистра; подкарточки одного глагола под это
// попадают автоматически (перевод у них общий). Это отношение эквивалентности, поэтому
// задача — классическая «переставь так, чтобы соседние ключи не совпадали»:
//   - обычно берём следующий ключ в shuffle-порядке, кроме совпадающего с предыдущей карточкой —
//     так сохраняется случайность;
//   - но если самая большая группа занимает ≥ половины оставшегося, ставим её сейчас,
//     иначе в хвосте она останется одна и соседства не избежать.
//
// Простой обмен «с ближайшей непохожей впереди» тут не годится: он застревает, когда
// в конце корзины остаются только подкарточки одного глагола.
// Корзины не перемешиваются; если развести невозможно (в корзине один глагол) — best effort.
func spreadSimilar(buckets [][]Card) []Card {
	out := []Card{}
	last, hasLast := "", false
	for _, bucket := range buckets {
		var keys []string // в порядке первого появления = shuffle-порядок
		groups := map[string][]Card{}
		for _, c := range bucket {
			k := similarKey(c)
			if _, ok := groups[k]; !ok {
				keys = append(keys, k)
			}
			groups[k] = append(groups[k], c)
		}

		for remaining := len(bucket); remaining > 0; remaining-- {
			first, biggest := "", ""
			for _, k := range keys {
				if len(groups[k]) == 0 || (hasLast && k == last) {
					continue
				}
				if first == "" {
					first = k
				}
				if biggest == "" || len(groups[k]) > len(groups[biggest]) {
					biggest = k
				}
			}
			pick := first
			switch {
			case first == "": // остался только ключ предыдущей карточки — соседства не избежать
				pick = last
			case 2*len(groups[biggest]) >= remaining:
				pick = biggest
			}
			out = append(out, groups[pick][0])
			groups[pick] = groups[pick][1:]
			last, hasLast = pick, true
		}
	}
	return out
}

func similarKey(c Card) string { return strings.ToLower(c.Translation) }

func similar(a, b Card) bool {
	return a.WordID == b.WordID || similarKey(a) == similarKey(b)
}

func shuffle[T any](s []T, r *rand.Rand) {
	r.Shuffle(len(s), func(i, j int) { s[i], s[j] = s[j], s[i] })
}

// RequeueOffset — через сколько карточек вернуть «Не знаю» (3–5, §7).
func RequeueOffset(r *rand.Rand) int {
	return 3 + r.IntN(3)
}
