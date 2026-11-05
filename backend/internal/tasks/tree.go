package tasks

import (
	"fmt"
	"slices"
	"time"

	"lexis/internal/platform/days"
	"lexis/internal/platform/httpx"
)

// Подзадачи (SPEC §18.3, B2). Чистые функции: свёртка дерева, проверка шагов, место шагов в колонке.

const (
	MaxDepth = 10 // уровней дерева: глубже уже не «шаги», а потеря фокуса
	MaxSteps = 30 // шагов за одно разбиение
)

// Node — дело в дереве для расчётов. Parent == 0 — корень.
type Node struct {
	ID      int64
	Parent  int64
	Blocks  int        // своя оценка
	DoneAt  *time.Time // осмысленно только у листа
	WorkSec int        // своё время в фокусе (сессии по этому делу)
}

// Agg — вычисленное по дереву.
type Agg struct {
	Leaf    bool
	DoneAt  *time.Time // лист — своя дата; родитель — когда закрыт последний лист (nil — не всё сделано)
	Blocks  int        // лист — свой объём; родитель — Σ объёмов листьев
	WorkSec int        // в фокусе: своё + всех шагов (сессии бывают и у дела, которое потом разбили)
	Depth   int        // корень = 1
	Root    int64      // id корня дерева
}

func (a Agg) Done() bool { return a.DoneAt != nil }

// Rollup считает Agg для всех дел. Листья — без детей; у родителя done и объём берутся
// только из листьев (собственная оценка родителя — лишь первоначальный ориентир).
// Цикл в данных (его быть не должно) не вешает расчёт: узлы цикла получают Depth 0.
func Rollup(nodes []Node) map[int64]Agg {
	byID := make(map[int64]Node, len(nodes))
	children := make(map[int64][]int64)
	for _, n := range nodes {
		byID[n.ID] = n
		children[n.Parent] = append(children[n.Parent], n.ID)
	}
	out := make(map[int64]Agg, len(nodes))
	var walk func(id, root int64, depth int) Agg
	walk = func(id, root int64, depth int) Agg {
		kids := children[id]
		if len(kids) == 0 {
			a := Agg{Leaf: true, DoneAt: byID[id].DoneAt, Blocks: byID[id].Blocks, WorkSec: byID[id].WorkSec, Depth: depth, Root: root}
			out[id] = a
			return a
		}
		a := Agg{Depth: depth, Root: root, WorkSec: byID[id].WorkSec}
		allDone := true
		var last *time.Time
		for _, k := range kids {
			c := walk(k, root, depth+1)
			a.Blocks += c.Blocks
			a.WorkSec += c.WorkSec
			if c.DoneAt == nil {
				allDone = false
			} else if last == nil || c.DoneAt.After(*last) {
				last = c.DoneAt
			}
		}
		if allDone {
			a.DoneAt = last
		}
		out[id] = a
		return a
	}
	for _, r := range children[0] {
		walk(r, r, 1)
	}
	return out
}

// Step — шаг в теле POST /api/tasks/{id}/split. Не задано (null/нет поля) — берётся у родителя.
type Step struct {
	Title      string  `json:"title"`
	Blocks     int     `json:"blocks"`
	Importance *int    `json:"importance"`
	Deadline   *string `json:"deadline"`
	TypeID     *int64  `json:"typeId"`
}

type SplitInput struct {
	Steps []Step `json:"steps"`
}

// Normalize проверяет шаги и наследует от родителя важность, дедлайн и тип (§18.3: «по умолчанию
// от родителя, редактируются»). Объём у шага свой: родитель потом считается как сумма шагов.
func (in SplitInput) Normalize(parent Fields) ([]Fields, error) {
	if err := in.checkCount(); err != nil {
		return nil, err
	}
	out := make([]Fields, len(in.Steps))
	for i, st := range in.Steps {
		f := Fields{Title: st.Title, Blocks: st.Blocks, Importance: parent.Importance, Deadline: parent.Deadline, TypeID: parent.TypeID}
		if st.Importance != nil {
			f.Importance = *st.Importance
		}
		if st.Deadline != nil {
			d, err := days.Parse(*st.Deadline)
			if err != nil {
				return nil, err
			}
			f.Deadline = &d
		}
		if st.TypeID != nil {
			f.TypeID = st.TypeID
		}
		f, err := f.check()
		if err != nil {
			return nil, err
		}
		out[i] = f
	}
	return out, nil
}

// checkCount — отдельно от Normalize, чтобы отсечь пустой запрос до транзакции (родитель ещё не прочитан).
func (in SplitInput) checkCount() error {
	if len(in.Steps) == 0 || len(in.Steps) > MaxSteps {
		return httpx.BadRequest("invalid_steps", fmt.Sprintf("steps must contain 1..%d items", MaxSteps))
	}
	return nil
}

// CheckSplit — можно ли добавить шаги к делу с этим Agg: выполненный лист не разбиваем
// (сначала «вернуть в работу»), глубина после разбиения ≤ MaxDepth.
func CheckSplit(a Agg) error {
	if a.Leaf && a.Done() {
		return httpx.Conflict("task_done", "a done task cannot be split: mark it undone first")
	}
	if a.Depth+1 > MaxDepth {
		return httpx.BadRequest("task_too_deep", fmt.Sprintf("tasks can be nested at most %d levels deep", MaxDepth))
	}
	return nil
}

// PlaceAfter — порядок колонки, где шаги встают сразу после родителя (родитель на доске
// скрыт, и шаги оказываются там, где он был). Родителя нет в колонке — шаги в конец.
func PlaceAfter(ids []int64, parent int64, steps []int64) []int64 {
	i := slices.Index(ids, parent)
	if i < 0 {
		return append(slices.Clone(ids), steps...)
	}
	return slices.Insert(slices.Clone(ids), i+1, steps...)
}
