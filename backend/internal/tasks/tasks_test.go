package tasks

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"lexis/internal/platform/clock"
	"lexis/internal/platform/httpx"
)

func code(err error) string {
	var he *httpx.Error
	if errors.As(err, &he) {
		return he.Code
	}
	if err != nil {
		return "unexpected: " + err.Error()
	}
	return ""
}

func TestTypeInput_Normalize(t *testing.T) {
	tests := []struct {
		name string
		in   TypeInput
		want TypeInput
		code string
	}{
		{"trim and collapse", TypeInput{Name: "  Учёба   и  работа "}, TypeInput{Name: "Учёба и работа"}, ""},
		{"color lower-cased", TypeInput{Name: "x", Color: new("#AABBCC")}, TypeInput{Name: "x", Color: new("#aabbcc")}, ""},
		{"empty name", TypeInput{Name: "   "}, TypeInput{}, "invalid_type_name"},
		{"40 runes ok", TypeInput{Name: strings.Repeat("я", 40)}, TypeInput{Name: strings.Repeat("я", 40)}, ""},
		{"41 runes", TypeInput{Name: strings.Repeat("я", 41)}, TypeInput{}, "invalid_type_name"},
		{"bad color", TypeInput{Name: "x", Color: new("red")}, TypeInput{}, "invalid_color"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.in.Normalize()
			if code(err) != tt.code {
				t.Fatalf("code = %q, want %q", code(err), tt.code)
			}
			if tt.code == "" && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestTypePatch_Apply(t *testing.T) {
	cur := TypeInput{Name: "Учёба", Color: new("#111111")}
	got, err := TypePatch{Color: httpx.Optional[string]{Set: true}}.Apply(cur)
	if err != nil || got.Name != "Учёба" || got.Color != nil {
		t.Errorf("clear color: %+v %v", got, err)
	}
	got, err = TypePatch{Name: new(" Жизнь ")}.Apply(cur)
	if err != nil || got.Name != "Жизнь" || *got.Color != "#111111" {
		t.Errorf("rename: %+v %v", got, err)
	}
	if _, err := (TypePatch{Name: new("")}).Apply(cur); code(err) != "invalid_type_name" {
		t.Errorf("empty rename: %v", err)
	}
}

func TestColumnName(t *testing.T) {
	if got, err := ColumnName("  В   работе "); err != nil || got != "В работе" {
		t.Errorf("got %q %v", got, err)
	}
	for _, bad := range []string{"", "  ", strings.Repeat("x", 41)} {
		if _, err := ColumnName(bad); code(err) != "invalid_column_name" {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

func TestTaskInput_Normalize(t *testing.T) {
	valid := func() TaskInput {
		return TaskInput{Title: " Написать  отчёт ", Blocks: 2, Importance: 7, ColumnID: 1, Note: "  заметка \n"}
	}
	got, err := valid().Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if want := (Fields{Title: "Написать отчёт", Blocks: 2, Importance: 7, Note: "заметка"}); !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}

	in := valid()
	in.Deadline = new("2026-11-10")
	got, err = in.Normalize()
	if err != nil || clock.FormatDate(*got.Deadline) != "2026-11-10" {
		t.Errorf("deadline: %+v %v", got.Deadline, err)
	}

	tests := []struct {
		name   string
		mutate func(*TaskInput)
		code   string
	}{
		{"past deadline is fine", func(in *TaskInput) { in.Deadline = new("2020-01-01") }, ""},
		{"empty title", func(in *TaskInput) { in.Title = "  " }, "invalid_title"},
		{"long title", func(in *TaskInput) { in.Title = strings.Repeat("я", 201) }, "invalid_title"},
		{"zero blocks", func(in *TaskInput) { in.Blocks = 0 }, "invalid_blocks"},
		{"41 blocks", func(in *TaskInput) { in.Blocks = 41 }, "invalid_blocks"},
		{"int16 overflow is not wrapped", func(in *TaskInput) { in.Blocks = 65538 }, "invalid_blocks"},
		{"importance 0", func(in *TaskInput) { in.Importance = 0 }, "invalid_importance"},
		{"importance 11", func(in *TaskInput) { in.Importance = 11 }, "invalid_importance"},
		{"bad deadline", func(in *TaskInput) { in.Deadline = new("завтра") }, "invalid_date"},
		{"no column", func(in *TaskInput) { in.ColumnID = 0 }, "invalid_column"},
		{"bad type id", func(in *TaskInput) { in.TypeID = new(int64(-1)) }, "invalid_type"},
		{"long note", func(in *TaskInput) { in.Note = strings.Repeat("я", MaxNoteLen+1) }, "text_too_long"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := valid()
			tt.mutate(&in)
			_, err := in.Normalize()
			if code(err) != tt.code {
				t.Errorf("code = %q, want %q", code(err), tt.code)
			}
		})
	}
}

func TestPatch_Apply(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	earlier := now.Add(-time.Hour)
	deadline := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	cur := Fields{Title: "Отчёт", Blocks: 2, Importance: 5, Deadline: &deadline, TypeID: new(int64(3)), Note: "x"}

	tests := []struct {
		name       string
		p          Patch
		doneAt     *time.Time
		want       Fields
		wantDoneAt *time.Time
		code       string
	}{
		{"empty patch keeps everything", Patch{}, nil, cur, nil, ""},
		{"partial", Patch{Blocks: new(4), Title: new(" Отчёт v2 ")}, nil,
			Fields{Title: "Отчёт v2", Blocks: 4, Importance: 5, Deadline: &deadline, TypeID: new(int64(3)), Note: "x"}, nil, ""},
		{"clear deadline and type", Patch{Deadline: httpx.Optional[string]{Set: true}, TypeID: httpx.Optional[int64]{Set: true}}, nil,
			Fields{Title: "Отчёт", Blocks: 2, Importance: 5, Note: "x"}, nil, ""},
		{"done sets now", Patch{Done: new(true)}, nil, cur, &now, ""},
		{"done again keeps date", Patch{Done: new(true)}, &earlier, cur, &earlier, ""},
		{"undone clears", Patch{Done: new(false)}, &earlier, cur, nil, ""},
		{"bad importance", Patch{Importance: new(0)}, nil, Fields{}, nil, "invalid_importance"},
		{"bad deadline", Patch{Deadline: httpx.Optional[string]{Set: true, Value: new("x")}}, nil, Fields{}, nil, "invalid_date"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, doneAt, err := tt.p.Apply(cur, tt.doneAt, now)
			if code(err) != tt.code {
				t.Fatalf("code = %q, want %q", code(err), tt.code)
			}
			if tt.code != "" {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("fields = %+v, want %+v", got, tt.want)
			}
			if !reflect.DeepEqual(doneAt, tt.wantDoneAt) {
				t.Errorf("doneAt = %v, want %v", doneAt, tt.wantDoneAt)
			}
		})
	}
}

func TestReorder(t *testing.T) {
	tests := []struct {
		name   string
		ids    []int64
		id     int64
		before *int64
		want   []int64
		code   string
	}{
		{"from another column to end", []int64{1, 2}, 9, nil, []int64{1, 2, 9}, ""},
		{"from another column before first", []int64{1, 2}, 9, new(int64(1)), []int64{9, 1, 2}, ""},
		{"within column: down", []int64{1, 2, 3}, 1, new(int64(3)), []int64{2, 1, 3}, ""},
		{"within column: up", []int64{1, 2, 3}, 3, new(int64(1)), []int64{3, 1, 2}, ""},
		{"within column: to end", []int64{1, 2, 3}, 1, nil, []int64{2, 3, 1}, ""},
		{"into empty column", nil, 9, nil, []int64{9}, ""},
		{"before itself", []int64{1, 2}, 1, new(int64(1)), nil, "invalid_task"},
		{"before task from another column", []int64{1, 2}, 9, new(int64(7)), nil, "invalid_task"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orig := append([]int64(nil), tt.ids...)
			got, err := Reorder(tt.ids, tt.id, tt.before)
			if code(err) != tt.code {
				t.Fatalf("code = %q, want %q", code(err), tt.code)
			}
			if tt.code == "" && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(tt.ids, orig) {
				t.Errorf("input mutated: %v", tt.ids)
			}
		})
	}
}

func TestCheckOrder(t *testing.T) {
	existing := []int64{1, 2, 3}
	tests := []struct {
		ids []int64
		ok  bool
	}{
		{[]int64{3, 1, 2}, true},
		{[]int64{1, 2}, false},
		{[]int64{1, 2, 2}, false},
		{[]int64{1, 2, 4}, false},
		{[]int64{1, 2, 3, 4}, false},
	}
	for _, tt := range tests {
		if err := CheckOrder(tt.ids, existing); (err == nil) != tt.ok {
			t.Errorf("CheckOrder(%v) = %v, want ok=%v", tt.ids, err, tt.ok)
		}
	}
}
