// Package tasks — «Игра в жизнь», дела (SPEC §18.3): типы дел, колонки доски, дела.
// Позже сюда же — подзадачи (B2), план дня и топ-5 (B3).
//
// Чистые правила (валидация, перестановки) — в этом файле; БД — service.go; HTTP — module.go.
package tasks

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"lexis/internal/platform/clock"
	"lexis/internal/platform/days"
	"lexis/internal/platform/httpx"
)

const (
	MaxNameLen    = 40 // имя типа и колонки
	MaxTitleLen   = 200
	MaxNoteLen    = 20000
	MaxBlocks     = 40 // 20 часов: больше — явно стоит разбить на шаги (B2)
	MaxImportance = 10
)

// Type — тип дела (учёба, жизнь…).
type Type struct {
	ID       int64   `json:"id"`
	Name     string  `json:"name"`
	Color    *string `json:"color"`
	Position int32   `json:"position"`
}

// Column — колонка доски.
type Column struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Position int32  `json:"position"`
}

// Task — дело в API. Blocks, Done, DoneAt у родителя вычислены по листьям (Rollup).
type Task struct {
	ID         int64      `json:"id"`
	ParentID   *int64     `json:"parentId"`
	Leaf       bool       `json:"leaf"` // без шагов — только такие видны на доске и в плане
	Title      string     `json:"title"`
	Blocks     int        `json:"blocks"`    // по 30 минут; у родителя — Σ листьев
	OwnBlocks  int        `json:"ownBlocks"` // собственная оценка (у листа = Blocks)
	Importance int16      `json:"importance"`
	Deadline   *string    `json:"deadline"` // YYYY-MM-DD или null
	TypeID     *int64     `json:"typeId"`
	ColumnID   int64      `json:"columnId"`
	Position   int32      `json:"position"`
	Note       string     `json:"note"`
	Done       bool       `json:"done"`
	WorkSec    int        `json:"workSec"` // время в фокусе; у родителя — вместе с шагами
	DoneAt     *time.Time `json:"doneAt"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

// ---------- типы и колонки ----------

var colorRe = regexp.MustCompile(`^#[0-9a-f]{6}$`)

// TypeInput — тело POST /types; база для PATCH.
type TypeInput struct {
	Name  string  `json:"name"`
	Color *string `json:"color"`
}

// Normalize: имя — trim и схлопнутые пробелы (1..40), цвет — lower-case #rrggbb или null.
func (in TypeInput) Normalize() (TypeInput, error) {
	name, ok := normalizeName(in.Name)
	if !ok {
		return in, httpx.BadRequest("invalid_type_name", fmt.Sprintf("type name must be 1..%d characters", MaxNameLen))
	}
	in.Name = name
	if in.Color != nil {
		c := strings.ToLower(*in.Color)
		if !colorRe.MatchString(c) {
			return in, httpx.BadRequest("invalid_color", "color must be #rrggbb or null")
		}
		in.Color = &c
	}
	return in, nil
}

// TypePatch — тело PATCH /types/{id}. Нет поля — не менять, color: null — без цвета.
type TypePatch struct {
	Name  *string                `json:"name"`
	Color httpx.Optional[string] `json:"color"`
}

func (p TypePatch) Apply(cur TypeInput) (TypeInput, error) {
	if p.Name != nil {
		cur.Name = *p.Name
	}
	if p.Color.Set {
		cur.Color = p.Color.Value
	}
	return cur.Normalize()
}

// ColumnName — имя колонки: trim, схлопнутые пробелы, 1..40.
func ColumnName(raw string) (string, error) {
	name, ok := normalizeName(raw)
	if !ok {
		return "", httpx.BadRequest("invalid_column_name", fmt.Sprintf("column name must be 1..%d characters", MaxNameLen))
	}
	return name, nil
}

func normalizeName(raw string) (string, bool) {
	name := strings.Join(strings.Fields(raw), " ")
	return name, name != "" && utf8.RuneCountInString(name) <= MaxNameLen
}

// ---------- дела ----------

// TaskInput — тело POST /api/tasks.
type TaskInput struct {
	Title      string  `json:"title"`
	Blocks     int     `json:"blocks"`
	Importance int     `json:"importance"`
	Deadline   *string `json:"deadline"` // YYYY-MM-DD или null
	TypeID     *int64  `json:"typeId"`
	ColumnID   int64   `json:"columnId"`
	Note       string  `json:"note"`
}

// Fields — проверенные поля дела (без колонки и порядка: ими занимается Move).
// Числа — int, а не int16 как в БД: проверяем исходное значение из JSON, иначе 70000
// при конвертации переполнилось бы и могло стать «допустимым».
type Fields struct {
	Title      string
	Blocks     int
	Importance int
	Deadline   *time.Time // гражданская дата (clock.DateOf), nil — без дедлайна
	TypeID     *int64
	Note       string
}

// Normalize проверяет новое дело. Дедлайн в прошлом допустим: это просто просроченное дело.
func (in TaskInput) Normalize() (Fields, error) {
	if in.ColumnID <= 0 {
		return Fields{}, httpx.BadRequest("invalid_column", "columnId must be a positive id")
	}
	var deadline *time.Time
	if in.Deadline != nil {
		d, err := days.Parse(*in.Deadline)
		if err != nil {
			return Fields{}, err
		}
		deadline = &d
	}
	return Fields{Title: in.Title, Blocks: in.Blocks, Importance: in.Importance,
		Deadline: deadline, TypeID: in.TypeID, Note: in.Note}.check()
}

// check — общие правила для создания и правки.
func (f Fields) check() (Fields, error) {
	f.Title = strings.Join(strings.Fields(f.Title), " ")
	if f.Title == "" || utf8.RuneCountInString(f.Title) > MaxTitleLen {
		return f, httpx.BadRequest("invalid_title", fmt.Sprintf("title must be 1..%d characters", MaxTitleLen))
	}
	if f.Blocks < 1 || f.Blocks > MaxBlocks {
		return f, httpx.BadRequest("invalid_blocks", fmt.Sprintf("blocks must be 1..%d", MaxBlocks))
	}
	if f.Importance < 1 || f.Importance > MaxImportance {
		return f, httpx.BadRequest("invalid_importance", fmt.Sprintf("importance must be 1..%d", MaxImportance))
	}
	if f.TypeID != nil && *f.TypeID <= 0 {
		return f, httpx.BadRequest("invalid_type", "typeId must be a positive id or null")
	}
	f.Note = strings.TrimSpace(f.Note)
	if utf8.RuneCountInString(f.Note) > MaxNoteLen {
		return f, httpx.BadRequest("text_too_long", fmt.Sprintf("note is longer than %d characters", MaxNoteLen))
	}
	return f, nil
}

// Patch — тело PATCH /api/tasks/{id}. Нет поля — не менять; deadline/typeId: null — убрать.
// done: true — выполнено (дата — сейчас), false — вернуть в работу.
type Patch struct {
	Title      *string                `json:"title"`
	Blocks     *int                   `json:"blocks"`
	Importance *int                   `json:"importance"`
	Deadline   httpx.Optional[string] `json:"deadline"`
	TypeID     httpx.Optional[int64]  `json:"typeId"`
	Note       *string                `json:"note"`
	Done       *bool                  `json:"done"`
}

// Apply накладывает патч на текущие поля. Повторное done: true дату выполнения не сдвигает.
func (p Patch) Apply(f Fields, doneAt *time.Time, now time.Time) (Fields, *time.Time, error) {
	if p.Title != nil {
		f.Title = *p.Title
	}
	if p.Blocks != nil {
		f.Blocks = *p.Blocks
	}
	if p.Importance != nil {
		f.Importance = *p.Importance
	}
	if p.Deadline.Set {
		f.Deadline = nil
		if p.Deadline.Value != nil {
			d, err := days.Parse(*p.Deadline.Value)
			if err != nil {
				return f, doneAt, err
			}
			f.Deadline = &d
		}
	}
	if p.TypeID.Set {
		f.TypeID = p.TypeID.Value
	}
	if p.Note != nil {
		f.Note = *p.Note
	}
	if p.Done != nil {
		switch {
		case !*p.Done:
			doneAt = nil
		case doneAt == nil:
			doneAt = &now
		}
	}
	f, err := f.check()
	return f, doneAt, err
}

// ---------- порядок ----------

// Reorder — новый порядок колонки: id убирается со старого места (если был в колонке)
// и встаёт перед before (nil — в конец). before должен быть в колонке и не равен id.
func Reorder(ids []int64, id int64, before *int64) ([]int64, error) {
	out := slices.DeleteFunc(slices.Clone(ids), func(x int64) bool { return x == id })
	if before == nil {
		return append(out, id), nil
	}
	i := slices.Index(out, *before)
	if i < 0 {
		return nil, httpx.BadRequest("invalid_task", "beforeId must be another task in the target column")
	}
	return slices.Insert(out, i, id), nil
}

// CheckOrder: новый порядок — перестановка всех существующих id (без лишних и дублей).
func CheckOrder(ids, existing []int64) error {
	bad := httpx.BadRequest("invalid_order", "ids must list every item exactly once")
	if len(ids) != len(existing) {
		return bad
	}
	want := make(map[int64]bool, len(existing))
	for _, id := range existing {
		want[id] = true
	}
	for _, id := range ids {
		if !want[id] {
			return bad
		}
		delete(want, id) // второй раз тот же id уже не найдётся
	}
	return nil
}

// formatDeadline — дата для API.
func formatDeadline(d *time.Time) *string {
	if d == nil {
		return nil
	}
	s := clock.FormatDate(*d)
	return &s
}
