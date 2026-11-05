package tasks

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"lexis/internal/platform/clock"
)

func TestRollup(t *testing.T) {
	t1 := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)
	//	1 курсовая (own 6)
	//	├── 2 глава 1 (own 2)
	//	│   ├── 4 черновик (3, done t2)
	//	│   └── 5 правка   (1, done t1)
	//	└── 3 глава 2 (4, open)
	//	6 шкаф (2, done t1) — отдельное дерево
	nodes := []Node{
		{ID: 1, Blocks: 6, WorkSec: 600}, // работали над курсовой до того, как разбили
		{ID: 2, Parent: 1, Blocks: 2},
		{ID: 3, Parent: 1, Blocks: 4, WorkSec: 1800},
		{ID: 4, Parent: 2, Blocks: 3, DoneAt: &t2, WorkSec: 60},
		{ID: 5, Parent: 2, Blocks: 1, DoneAt: &t1},
		{ID: 6, Blocks: 2, DoneAt: &t1},
	}
	got := Rollup(nodes)
	want := map[int64]Agg{
		1: {Leaf: false, DoneAt: nil, Blocks: 8, WorkSec: 2460, Depth: 1, Root: 1}, // глава 2 не сделана; own 6 не учитывается
		2: {Leaf: false, DoneAt: &t2, Blocks: 4, WorkSec: 60, Depth: 2, Root: 1},   // оба шага сделаны — дата последнего
		3: {Leaf: true, DoneAt: nil, Blocks: 4, WorkSec: 1800, Depth: 2, Root: 1},
		4: {Leaf: true, DoneAt: &t2, Blocks: 3, WorkSec: 60, Depth: 3, Root: 1},
		5: {Leaf: true, DoneAt: &t1, Blocks: 1, Depth: 3, Root: 1},
		6: {Leaf: true, DoneAt: &t1, Blocks: 2, Depth: 1, Root: 6},
	}
	if !reflect.DeepEqual(got, want) {
		for id := range want {
			if !reflect.DeepEqual(got[id], want[id]) {
				t.Errorf("task %d: got %+v, want %+v", id, got[id], want[id])
			}
		}
	}
	// Цикл в данных не вешает расчёт: такие узлы просто не попадают в результат.
	cyc := Rollup([]Node{{ID: 1, Parent: 2}, {ID: 2, Parent: 1}})
	if len(cyc) != 0 {
		t.Errorf("cycle: %+v", cyc)
	}
}

func TestSplitInput_Normalize(t *testing.T) {
	deadline := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	parent := Fields{Title: "Курсовая", Blocks: 12, Importance: 8, Deadline: &deadline, TypeID: new(int64(3)), Note: "x"}

	got, err := SplitInput{Steps: []Step{
		{Title: " Глава  1 ", Blocks: 4},
		{Title: "Правки", Blocks: 2, Importance: new(5), Deadline: new("2026-11-15"), TypeID: new(int64(7))},
	}}.Normalize(parent)
	if err != nil {
		t.Fatal(err)
	}
	// первый шаг унаследовал важность, дедлайн и тип; заметку — нет
	if s := got[0]; s.Title != "Глава 1" || s.Blocks != 4 || s.Importance != 8 || !s.Deadline.Equal(deadline) || *s.TypeID != 3 || s.Note != "" {
		t.Errorf("inherited step: %+v", s)
	}
	if s := got[1]; s.Importance != 5 || clock.FormatDate(*s.Deadline) != "2026-11-15" || *s.TypeID != 7 {
		t.Errorf("own step: %+v", s)
	}

	tooMany := make([]Step, MaxSteps+1)
	for i := range tooMany {
		tooMany[i] = Step{Title: "x", Blocks: 1}
	}
	tests := []struct {
		name  string
		steps []Step
		code  string
	}{
		{"no steps", nil, "invalid_steps"},
		{"too many", tooMany, "invalid_steps"},
		{"empty title", []Step{{Title: " ", Blocks: 1}}, "invalid_title"},
		{"no blocks", []Step{{Title: "x"}}, "invalid_blocks"},
		{"bad importance", []Step{{Title: "x", Blocks: 1, Importance: new(11)}}, "invalid_importance"},
		{"bad deadline", []Step{{Title: "x", Blocks: 1, Deadline: new("потом")}}, "invalid_date"},
		{"long title", []Step{{Title: strings.Repeat("я", 201), Blocks: 1}}, "invalid_title"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := SplitInput{Steps: tt.steps}.Normalize(parent)
			if code(err) != tt.code {
				t.Errorf("code = %q, want %q", code(err), tt.code)
			}
		})
	}
}

func TestCheckSplit(t *testing.T) {
	done := time.Now()
	tests := []struct {
		name string
		a    Agg
		code string
	}{
		{"open leaf", Agg{Leaf: true, Depth: 1}, ""},
		{"parent: add more steps", Agg{Depth: 3}, ""},
		{"done parent: still can add steps", Agg{DoneAt: &done, Depth: 1}, ""},
		{"done leaf", Agg{Leaf: true, DoneAt: &done, Depth: 1}, "task_done"},
		{"max depth leaf", Agg{Leaf: true, Depth: MaxDepth}, "task_too_deep"},
		{"one level below max", Agg{Leaf: true, Depth: MaxDepth - 1}, ""},
	}
	for _, tt := range tests {
		if got := code(CheckSplit(tt.a)); got != tt.code {
			t.Errorf("%s: code = %q, want %q", tt.name, got, tt.code)
		}
	}
}

func TestPlaceAfter(t *testing.T) {
	tests := []struct {
		name   string
		ids    []int64
		parent int64
		steps  []int64
		want   []int64
	}{
		{"middle", []int64{1, 2, 3}, 2, []int64{8, 9}, []int64{1, 2, 8, 9, 3}},
		{"last", []int64{1, 2}, 2, []int64{9}, []int64{1, 2, 9}},
		{"parent elsewhere", []int64{1, 2}, 7, []int64{9}, []int64{1, 2, 9}},
	}
	for _, tt := range tests {
		ids := append([]int64(nil), tt.ids...)
		if got := PlaceAfter(tt.ids, tt.parent, tt.steps); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
		if !reflect.DeepEqual(ids, tt.ids) {
			t.Errorf("%s: input mutated", tt.name)
		}
	}
}
