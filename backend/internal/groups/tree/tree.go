// Package tree — дерево групп английского (SPEC §18.2): глубина, поддерево, проверки переноса и пути из тега.
//
// Чистый код без БД: сервис грузит все группы (их десятки) одним запросом и проверяет
// изменения здесь. Отдельный пакет, а не groups: дерево нужно и words (теги с путём),
// а groups сам импортирует words — был бы цикл импортов.
package tree

import (
	"errors"
	"fmt"
)

// MaxDepth — сколько уровней можно: корень → подгруппа → под-подгруппа.
const MaxDepth = 3

// PathSep — разделитель пути в теге: #путешествия/аэропорт.
const PathSep = "/"

// LockKey — ключ pg_advisory_xact_lock для всех изменений дерева (перенос, создание по пути).
// Проверка «нет цикла и глубина ок» читает всё дерево — без блокировки два параллельных
// переноса могли бы вместе собрать цикл, хотя каждый по отдельности корректен.
const LockKey int64 = 0x6c657869735f6772 // "lexis_gr"

var (
	ErrNotFound     = errors.New("group not found")
	ErrCycle        = errors.New("group cannot be moved into itself or its descendant")
	ErrTooDeep      = errors.New("group tree is too deep")
	ErrPathConflict = errors.New("group already exists under another parent")
)

// Node — группа в дереве. Parent == 0 — корень (id групп начинаются с 1).
type Node struct {
	ID     int64
	Name   string
	Parent int64
}

type Tree struct {
	nodes    map[int64]Node
	byName   map[string]int64
	children map[int64][]int64
}

func New(nodes []Node) *Tree {
	t := &Tree{
		nodes:    make(map[int64]Node, len(nodes)),
		byName:   make(map[string]int64, len(nodes)),
		children: make(map[int64][]int64),
	}
	for _, n := range nodes {
		t.Add(n)
	}
	return t
}

// Add добавляет узел (например, только что созданную группу).
func (t *Tree) Add(n Node) {
	t.nodes[n.ID] = n
	t.byName[n.Name] = n.ID
	t.children[n.Parent] = append(t.children[n.Parent], n.ID)
}

func (t *Tree) Has(id int64) bool {
	_, ok := t.nodes[id]
	return ok
}

// ByName — id группы по нормализованному имени (имена глобально уникальны).
func (t *Tree) ByName(name string) (int64, bool) {
	id, ok := t.byName[name]
	return id, ok
}

// Parent — id родителя; 0 — корень.
func (t *Tree) Parent(id int64) int64 { return t.nodes[id].Parent }

// Depth — уровень группы: корневая = 1. Шагов не больше числа узлов — защита от цикла
// в данных (в БД его быть не должно, но бесконечный цикл хуже неверной глубины).
func (t *Tree) Depth(id int64) int {
	d := 0
	for cur := id; cur != 0 && d <= len(t.nodes); cur = t.nodes[cur].Parent {
		d++
	}
	return d
}

// Height — число уровней в поддереве: лист = 1.
func (t *Tree) Height(id int64) int {
	h := 0
	for _, c := range t.children[id] {
		h = max(h, t.Height(c))
	}
	return h + 1
}

// Subtree — сама группа и все потомки (в порядке обхода в ширину).
func (t *Tree) Subtree(id int64) []int64 {
	ids := []int64{id}
	for i := 0; i < len(ids); i++ {
		ids = append(ids, t.children[ids[i]]...)
	}
	return ids
}

// Children — прямые подгруппы.
func (t *Tree) Children(id int64) []int64 { return t.children[id] }

// IsDescendant — of лежит в поддереве id (включая сам id).
func (t *Tree) IsDescendant(of, id int64) bool {
	for cur := of; cur != 0; cur = t.nodes[cur].Parent {
		if cur == id {
			return true
		}
	}
	return false
}

// CheckMove проверяет перенос id под newParent (0 — в корень): не в себя и не в потомка,
// и после переноса самое глубокое место поддерева не глубже MaxDepth.
func (t *Tree) CheckMove(id, newParent int64) error {
	if !t.Has(id) || (newParent != 0 && !t.Has(newParent)) {
		return ErrNotFound
	}
	if newParent != 0 && t.IsDescendant(newParent, id) {
		return ErrCycle
	}
	if t.Depth(newParent)+t.Height(id) > MaxDepth { // Depth(0) = 0
		return ErrTooDeep
	}
	return nil
}

// CheckNew — можно ли создать группу под parent (0 — в корень).
func (t *Tree) CheckNew(parent int64) error {
	if parent != 0 && !t.Has(parent) {
		return ErrNotFound
	}
	if t.Depth(parent)+1 > MaxDepth {
		return ErrTooDeep
	}
	return nil
}

// CheckPath проверяет путь из тега (части уже нормализованы, без повторов — это гарантирует парсер).
// Путь говорит «каждая часть — дочерняя для предыдущей»; первая часть может лежать где угодно.
// Существующая группа с другим родителем не переносится — ErrPathConflict (явная разметка,
// без догадок: §18.2). Новые части создадут по цепочке — проверяем, что они влезут по глубине.
func (t *Tree) CheckPath(parts []string) error {
	var prev int64 // id предыдущей части; 0 — её ещё нет (будет создана)
	depth := 0
	for i, name := range parts {
		id, exists := t.byName[name]
		switch {
		// prev == 0 тоже конфликт: под новой группой существующей быть не может, а переносить её не будем.
		// Отдельно от сравнения с Parent: у корневой группы Parent тоже 0.
		case exists && i > 0 && (prev == 0 || t.nodes[id].Parent != prev):
			return ErrPathConflict
		case exists:
			depth = t.Depth(id)
		default:
			depth++
		}
		if depth > MaxDepth {
			return ErrTooDeep
		}
		prev = 0
		if exists {
			prev = id
		}
	}
	return nil
}

// CheckPaths — CheckPath для всех путей карточки по очереди, как будто предыдущие пути уже
// создали свои группы: #x/y #z/y по отдельности годятся, а вместе — конфликт (y уже под x).
// Проверяем на копии с фиктивными отрицательными id, чтобы потом создавать без сюрпризов.
func (t *Tree) CheckPaths(paths [][]string) error {
	sim := New(nil)
	for _, n := range t.nodes {
		sim.Add(n)
	}
	next := int64(-1)
	for _, p := range paths {
		if err := sim.CheckPath(p); err != nil {
			return err
		}
		var parent int64
		for _, name := range p {
			id, ok := sim.byName[name]
			if !ok {
				id, next = next, next-1
				sim.Add(Node{ID: id, Name: name, Parent: parent})
			}
			parent = id
		}
	}
	return nil
}

// Validate проверяет дерево из внешнего источника (файл бэкапа): у каждого узла родитель
// существует, циклов нет, глубина ≤ MaxDepth. Возвращает ErrNotFound / ErrCycle / ErrTooDeep.
func (t *Tree) Validate() error {
	for id, n := range t.nodes {
		if n.Parent != 0 && !t.Has(n.Parent) {
			return fmt.Errorf("group %d: parent %d: %w", id, n.Parent, ErrNotFound)
		}
		depth := 0
		for cur := id; cur != 0; cur = t.nodes[cur].Parent {
			if depth++; depth > MaxDepth {
				// Цикл тоже упирается сюда (бесконечная глубина): различаем, дойдёт ли подъём до корня.
				if !t.reachesRoot(id) {
					return fmt.Errorf("group %d: %w", id, ErrCycle)
				}
				return fmt.Errorf("group %d: %w", id, ErrTooDeep)
			}
		}
	}
	return nil
}

// reachesRoot — подъём от id по родителям доходит до корня. Больше len(nodes) шагов —
// значит, ходим по кругу (id на цикле или под ним).
func (t *Tree) reachesRoot(id int64) bool {
	cur := id
	for range len(t.nodes) + 1 {
		if cur == 0 {
			return true
		}
		cur = t.nodes[cur].Parent
	}
	return cur == 0
}
