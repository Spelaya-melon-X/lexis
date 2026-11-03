package training

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"
	"time"

	"lexis/internal/words"
)

// Порядок внутри корзин случайный, поэтому большинство тестов проверяют свойства
// очереди на многих seed, а не конкретную перестановку.
const seeds = 50

func rng(seed uint64) *rand.Rand { return rand.New(rand.NewPCG(seed, seed)) }

var qNow = at(10, 12, 0)

func word(id int64, level int16, next time.Time) QueueWord {
	return QueueWord{ID: id, Term: fmt.Sprintf("w%d", id), Translation: fmt.Sprintf("t%d", id), Level: level, NextReviewAt: next}
}

func examples(texts ...string) []words.Example {
	out := make([]words.Example, len(texts))
	for i, t := range texts {
		out[i], _ = words.ParseExample(t)
	}
	return out
}

func ids(cards []Card) []int64 {
	out := make([]int64, len(cards))
	for i, c := range cards {
		out[i] = c.WordID
	}
	return out
}

func TestBuild_Buckets(t *testing.T) {
	ws := []QueueWord{
		word(1, 2, at(8, 12, 0)),   // просрочено
		word(2, 3, at(9, 23, 59)),  // просрочено (вчера)
		word(3, 2, at(10, 0, 0)),   // сегодня, начало дня
		word(4, 4, at(10, 23, 59)), // сегодня, конец дня
		word(5, 1, at(10, 9, 0)),   // новое
		word(6, 1, at(1, 9, 0)),    // новое, хоть и давно создано
		word(7, 2, at(11, 0, 0)),   // завтра — не due
		word(8, 5, at(1, 0, 0)),    // архив — никогда
	}
	overdue, today, fresh := []int64{1, 2}, []int64{3, 4}, []int64{5, 6}

	for seed := range uint64(seeds) {
		got := ids(Build(ws, BuildOptions{Strategy: StrategyEnRu, Now: qNow, NewLimit: 10, Rand: rng(seed)}))
		if len(got) != 6 {
			t.Fatalf("seed %d: got %v, want 6 cards", seed, got)
		}
		for i, bucket := range [][]int64{overdue, today, fresh} {
			part := slices.Clone(got[i*2 : i*2+2])
			slices.Sort(part)
			if !reflect.DeepEqual(part, bucket) {
				t.Fatalf("seed %d: order %v, bucket %d want %v", seed, got, i, bucket)
			}
		}
	}
}

func TestBuild_NewLimit(t *testing.T) {
	var ws []QueueWord
	for id := int64(1); id <= 20; id++ {
		ws = append(ws, word(id, 1, at(1, 0, 0)))
	}
	ws = append(ws, word(100, 2, at(9, 0, 0))) // due-слово лимитом не режется

	tests := []struct {
		limit, wantNew int
	}{
		{10, 10}, {0, 0}, {25, 20}, {-1, 0},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("limit %d", tt.limit), func(t *testing.T) {
			got := ids(Build(ws, BuildOptions{Strategy: StrategyEnRu, Now: qNow, NewLimit: tt.limit, Rand: rng(1)}))
			if len(got) != tt.wantNew+1 || got[0] != 100 {
				t.Errorf("got %v, want due word first + %d new", got, tt.wantNew)
			}
		})
	}

	// Лимит берётся после shuffle: в разных сессиях — разные новые слова.
	seen := map[string]bool{}
	for seed := range uint64(seeds) {
		got := ids(Build(ws, BuildOptions{Strategy: StrategyEnRu, Now: qNow, NewLimit: 5, Rand: rng(seed)}))
		slices.Sort(got)
		seen[fmt.Sprint(got)] = true
	}
	if len(seen) < 2 {
		t.Errorf("new words selection never changes across seeds")
	}
}

func TestBuild_Free(t *testing.T) {
	ws := []QueueWord{
		word(1, 5, at(1, 0, 0)),  // архив — во free тоже
		word(2, 2, at(20, 0, 0)), // не due — во free тоже
		word(3, 1, at(1, 0, 0)),
		word(4, 3, at(5, 0, 0)),
	}
	orig := slices.Clone(ws)
	got := ids(Build(ws, BuildOptions{Strategy: StrategyEnRu, Now: qNow, NewLimit: 0, Free: true, Rand: rng(7)}))
	slices.Sort(got)
	if !reflect.DeepEqual(got, []int64{1, 2, 3, 4}) {
		t.Errorf("free must include every word, got %v", got)
	}
	if !reflect.DeepEqual(ws, orig) {
		t.Errorf("Build must not reorder caller's slice")
	}
}

func TestBuild_Expand(t *testing.T) {
	verb := QueueWord{ID: 1, Term: "go_went_gone", Forms: []string{"go", "went", "gone"}, Translation: "идти", Level: 2,
		NextReviewAt: at(9, 0, 0), Examples: examples("I [go]", "I [went]", "I have [gone]")}
	plain := QueueWord{ID: 2, Term: "apple", Translation: "яблоко", Level: 2,
		NextReviewAt: at(9, 0, 0), Examples: examples("An [apple]", "Red [apple]", "Green [apple]")}
	bare := QueueWord{ID: 3, Term: "pear", Translation: "груша", Level: 2, NextReviewAt: at(9, 0, 0)}

	tests := []struct {
		name     string
		strategy Strategy
		w        QueueWord
		want     []string // Text примера у каждой карточки, "" — без примера
		anyOf    bool     // want — варианты для единственной карточки
	}{
		{"en_ru ignores examples", StrategyEnRu, verb, []string{""}, false},
		{"ru_en ignores examples", StrategyRuEn, plain, []string{""}, false},
		{"context: verb → subcard per example", StrategyContext, verb, []string{"I [go]", "I [went]", "I have [gone]"}, false},
		{"context: plain word → one random example", StrategyContext, plain, []string{"An [apple]", "Red [apple]", "Green [apple]"}, true},
		{"context: no examples → card without example", StrategyContext, bare, []string{""}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cards := expand(tt.w, tt.strategy, rng(3))
			var got []string
			for _, c := range cards {
				if c.WordID != tt.w.ID || c.Translation != tt.w.Translation {
					t.Errorf("card fields: %+v", c)
				}
				if c.Example == nil {
					got = append(got, "")
				} else {
					got = append(got, c.Example.Text)
				}
			}
			if tt.anyOf {
				if len(got) != 1 || !slices.Contains(tt.want, got[0]) {
					t.Errorf("got %q, want one of %q", got, tt.want)
				}
				return
			}
			slices.Sort(got)
			want := slices.Clone(tt.want)
			slices.Sort(want)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}

	// Для обычного слова пример выбирается случайно — со временем выпадают разные.
	seen := map[string]bool{}
	for seed := range uint64(seeds) {
		seen[expand(plain, StrategyContext, rng(seed))[0].Example.Text] = true
	}
	if len(seen) < 2 {
		t.Errorf("random example never changes: %v", seen)
	}
}

func TestBuild_SpreadSimilar(t *testing.T) {
	verb := QueueWord{ID: 1, Term: "go_went_gone", Forms: []string{"go", "went", "gone"}, Translation: "идти", Level: 2,
		NextReviewAt: at(9, 0, 0), Examples: examples("I [go]", "I [went]", "I have [gone]")}
	ws := []QueueWord{
		verb,
		{ID: 2, Term: "walk", Translation: "Гулять", Level: 2, NextReviewAt: at(9, 0, 0)},
		{ID: 3, Term: "stroll", Translation: "гулять", Level: 2, NextReviewAt: at(9, 0, 0)}, // тот же перевод
		word(4, 2, at(9, 0, 0)),
		word(5, 2, at(9, 0, 0)),
		word(6, 2, at(9, 0, 0)),
	}
	for seed := range uint64(seeds) {
		cards := Build(ws, BuildOptions{Strategy: StrategyContext, Now: qNow, NewLimit: 10, Rand: rng(seed)})
		if len(cards) != 8 {
			t.Fatalf("seed %d: %d cards, want 8", seed, len(cards))
		}
		for i := 1; i < len(cards); i++ {
			if similar(cards[i-1], cards[i]) {
				t.Fatalf("seed %d: similar cards adjacent at %d: %v", seed, i, ids(cards))
			}
		}
	}
}

func TestBuild_SpreadIsBestEffort(t *testing.T) {
	// Единственный глагол в очереди — развести не с чем, но и паники/потерь нет.
	verb := QueueWord{ID: 1, Term: "go_went_gone", Forms: []string{"go", "went", "gone"}, Translation: "идти", Level: 2,
		NextReviewAt: at(9, 0, 0), Examples: examples("I [go]", "I [went]", "I have [gone]")}
	cards := Build([]QueueWord{verb}, BuildOptions{Strategy: StrategyContext, Now: qNow, Rand: rng(1)})
	if !reflect.DeepEqual(ids(cards), []int64{1, 1, 1}) {
		t.Errorf("got %v", ids(cards))
	}
}

func TestBuild_SpreadKeepsBuckets(t *testing.T) {
	// Похожие слова в разных корзинах: разнесение не тащит новое слово вперёд просроченных.
	ws := []QueueWord{
		{ID: 1, Term: "a", Translation: "same", Level: 2, NextReviewAt: at(9, 0, 0)}, // просрочено
		{ID: 2, Term: "b", Translation: "same", Level: 1, NextReviewAt: at(9, 0, 0)}, // новое
		{ID: 3, Term: "c", Translation: "other", Level: 1, NextReviewAt: at(9, 0, 0)},
	}
	for seed := range uint64(seeds) {
		got := ids(Build(ws, BuildOptions{Strategy: StrategyEnRu, Now: qNow, NewLimit: 10, Rand: rng(seed)}))
		if got[0] != 1 || got[1] != 3 || got[2] != 2 {
			t.Fatalf("seed %d: got %v, want [1 3 2]", seed, got)
		}
	}
}

func TestBuild_Deterministic(t *testing.T) {
	var ws []QueueWord
	for id := int64(1); id <= 15; id++ {
		ws = append(ws, word(id, int16(1+id%3), at(9, 0, 0)))
	}
	o := func() BuildOptions {
		return BuildOptions{Strategy: StrategyEnRu, Now: qNow, NewLimit: 3, Rand: rng(42)}
	}
	if a, b := ids(Build(ws, o())), ids(Build(ws, o())); !reflect.DeepEqual(a, b) {
		t.Errorf("same seed, different queues: %v vs %v", a, b)
	}
}

func TestBuild_Empty(t *testing.T) {
	got := Build(nil, BuildOptions{Strategy: StrategyEnRu, Now: qNow, NewLimit: 10, Rand: rng(1)})
	if got == nil || len(got) != 0 {
		t.Errorf("want empty non-nil slice, got %#v", got)
	}
}

func TestRequeueOffset(t *testing.T) {
	seen := map[int]bool{}
	r := rng(1)
	for range 200 {
		n := RequeueOffset(r)
		if n < 3 || n > 5 {
			t.Fatalf("offset %d out of 3..5", n)
		}
		seen[n] = true
	}
	if len(seen) != 3 {
		t.Errorf("want all of 3,4,5, got %v", seen)
	}
}

func TestStrategy_Valid(t *testing.T) {
	for _, s := range []Strategy{StrategyEnRu, StrategyRuEn, StrategyContext} {
		if !s.Valid() {
			t.Errorf("%q must be valid", s)
		}
	}
	if Strategy("en-ru").Valid() {
		t.Error("en-ru must be invalid")
	}
}
