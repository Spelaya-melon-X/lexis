//go:build integration

// Подзадачи (§18.3, B2) на живом Postgres: разбиение, свёртка, граф, удаление с шагами.
package tasks

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func titles(ts []Task) []string {
	out := make([]string, len(ts))
	for i, x := range ts {
		out[i] = x.Title
	}
	return out
}

func byTitle(t *testing.T, ts []Task, title string) Task {
	t.Helper()
	for _, x := range ts {
		if x.Title == title {
			return x
		}
	}
	t.Fatalf("task %q not found in %v", title, titles(ts))
	return Task{}
}

func TestSplit(t *testing.T) {
	s, clk := newTestService(t)
	ctx := context.Background()
	col := must(s.CreateColumn(ctx, "Входящие"))
	typ := must(s.CreateType(ctx, TypeInput{Name: "Учёба"}))
	paper := must(s.Create(ctx, TaskInput{Title: "Курсовая", Blocks: 6, Importance: 8, Deadline: new("2026-11-20"), TypeID: &typ.ID, ColumnID: col.ID}))
	other := must(s.Create(ctx, TaskInput{Title: "Шкаф", Blocks: 2, Importance: 3, ColumnID: col.ID}))

	tree := must(s.Split(ctx, paper.ID, SplitInput{Steps: []Step{
		{Title: "Глава 1", Blocks: 4},
		{Title: "Глава 2", Blocks: 2, Importance: new(5)},
	}}))
	if got := titles(tree); !reflect.DeepEqual(got, []string{"Курсовая", "Глава 1", "Глава 2"}) {
		t.Fatalf("tree = %v", got)
	}
	root, ch1 := tree[0], tree[1]
	if root.Leaf || root.Blocks != 6 || root.OwnBlocks != 6 || root.Done {
		t.Errorf("root: %+v", root)
	}
	if !ch1.Leaf || *ch1.ParentID != paper.ID || ch1.Importance != 8 || *ch1.Deadline != "2026-11-20" || *ch1.TypeID != typ.ID {
		t.Errorf("inherited step: %+v", ch1)
	}
	// Шаги встали в колонку сразу после родителя.
	if got := order(t, s, col.ID); !reflect.DeepEqual(got, []int64{paper.ID, ch1.ID, tree[2].ID, other.ID}) {
		t.Errorf("column order = %v", got)
	}

	_, err := s.Update(ctx, paper.ID, Patch{Done: new(true)})
	assertCode(t, err, 409, "task_not_leaf")

	// Родитель выполнен, когда выполнены все шаги; дата — последнего.
	must(s.Update(ctx, ch1.ID, Patch{Done: new(true)}))
	clk.Advance(time.Hour)
	must(s.Update(ctx, tree[2].ID, Patch{Done: new(true)}))
	done := must(s.Tasks(ctx, FilterDone))
	if r := byTitle(t, done, "Курсовая"); !r.Done || !r.DoneAt.Equal(testNow.Add(time.Hour)) {
		t.Errorf("rolled-up done: %+v", r)
	}
	if open := must(s.Tasks(ctx, FilterOpen)); !reflect.DeepEqual(titles(open), []string{"Шкаф"}) {
		t.Errorf("open = %v", titles(open))
	}

	// Выполненный лист не разбиваем; вернули в работу — можно, и родитель снова открыт.
	_, err = s.Split(ctx, ch1.ID, SplitInput{Steps: []Step{{Title: "x", Blocks: 1}}})
	assertCode(t, err, 409, "task_done")
	must(s.Update(ctx, ch1.ID, Patch{Done: new(false)}))
	tree = must(s.Split(ctx, ch1.ID, SplitInput{Steps: []Step{{Title: "Черновик", Blocks: 3}}}))
	if got := titles(tree); !reflect.DeepEqual(got, []string{"Курсовая", "Глава 1", "Черновик", "Глава 2"}) {
		t.Errorf("tree after nested split = %v", got)
	}
	if r := tree[0]; r.Done || r.Blocks != 5 { // Черновик 3 + Глава 2 (2); оценка «Главы 1» больше не считается
		t.Errorf("root after nested split: %+v", r)
	}
	// Граф одинаковый, с какого узла ни спроси.
	if got := titles(must(s.Tree(ctx, tree[2].ID))); !reflect.DeepEqual(got, titles(tree)) {
		t.Errorf("tree from a leaf = %v", got)
	}

	_, err = s.Split(ctx, 999, SplitInput{Steps: []Step{{Title: "x", Blocks: 1}}})
	assertCode(t, err, 404, "not_found")
	_, err = s.Tree(ctx, 999)
	assertCode(t, err, 404, "not_found")

	// Удаление корня — со всеми шагами.
	if err := s.Delete(ctx, paper.ID); err != nil {
		t.Fatal(err)
	}
	if all := must(s.Tasks(ctx, FilterAll)); !reflect.DeepEqual(titles(all), []string{"Шкаф"}) {
		t.Errorf("after delete: %v", titles(all))
	}
}

func TestSplit_MaxDepth(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	col := must(s.CreateColumn(ctx, "c"))
	cur := must(s.Create(ctx, TaskInput{Title: "1", Blocks: 1, Importance: 5, ColumnID: col.ID}))
	for depth := 2; depth <= MaxDepth; depth++ {
		tree := must(s.Split(ctx, cur.ID, SplitInput{Steps: []Step{{Title: "x", Blocks: 1}}}))
		cur = tree[len(tree)-1] // обход в глубину — новый шаг последний
	}
	_, err := s.Split(ctx, cur.ID, SplitInput{Steps: []Step{{Title: "x", Blocks: 1}}})
	assertCode(t, err, 400, "task_too_deep")
}
