package tasks

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lexis/internal/platform/clock"
	"lexis/internal/platform/db"
	"lexis/internal/platform/eventbus"
	"lexis/internal/platform/httpx"
	"lexis/internal/tasks/store"
)

type Service struct {
	pool  *pgxpool.Pool
	clock clock.Clock
	bus   eventbus.Publisher
}

// NewService: bus — для TaskCompleted (достижения «Игры», §18.3 B5).
func NewService(pool *pgxpool.Pool, clk clock.Clock, bus eventbus.Publisher) *Service {
	return &Service{pool: pool, clock: clk, bus: bus}
}

var (
	errTaskNotFound   = httpx.NotFound("task not found")
	errTypeNotFound   = httpx.NotFound("task type not found")
	errColumnNotFound = httpx.NotFound("column not found")
	errDuplicateType  = httpx.Conflict("duplicate_type", "a type with this name already exists")
	// Ссылки из тела запроса — 400, а не 404: не найден не ресурс из URL, а значение поля.
	errInvalidType   = httpx.BadRequest("invalid_type", "unknown task type")
	errInvalidColumn = httpx.BadRequest("invalid_column", "unknown column")
)

func toType(t store.TaskType) Type {
	return Type{ID: t.ID, Name: t.Name, Color: t.Color, Position: t.Position}
}

func toColumn(c store.TaskColumn) Column {
	return Column{ID: c.ID, Name: c.Name, Position: c.Position}
}

func toTask(t store.Task, a Agg) Task {
	return Task{
		ID: t.ID, ParentID: t.ParentID, Leaf: a.Leaf, Title: t.Title, Blocks: a.Blocks, OwnBlocks: int(t.Blocks),
		Importance: t.Importance, Deadline: formatDeadline(t.Deadline), TypeID: t.TypeID, ColumnID: t.ColumnID,
		Position: t.Position, Note: t.Note, Done: a.Done(), DoneAt: a.DoneAt, WorkSec: a.WorkSec, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}
}

// snapshot — все дела и свёртка дерева. Выполненность и объём родителя зависят от всего
// поддерева, а дел — сотни, поэтому для ответа с вычисляемыми полями грузим всё одним запросом.
type snapshot struct {
	rows []store.Task
	agg  map[int64]Agg
}

func loadSnapshot(ctx context.Context, q *store.Queries) (snapshot, error) {
	rows, err := q.ListTasks(ctx)
	if err != nil {
		return snapshot{}, fmt.Errorf("list tasks: %w", err)
	}
	work, err := q.TaskWorkSec(ctx)
	if err != nil {
		return snapshot{}, fmt.Errorf("task work: %w", err)
	}
	workOf := make(map[int64]int, len(work))
	for _, w := range work {
		workOf[w.TaskID] = int(w.WorkSec)
	}
	nodes := make([]Node, len(rows))
	for i, r := range rows {
		var parent int64
		if r.ParentID != nil {
			parent = *r.ParentID
		}
		nodes[i] = Node{ID: r.ID, Parent: parent, Blocks: int(r.Blocks), DoneAt: r.DoneAt, WorkSec: workOf[r.ID]}
	}
	return snapshot{rows: rows, agg: Rollup(nodes)}, nil
}

func (sn snapshot) task(id int64) (Task, bool) {
	for _, r := range sn.rows {
		if r.ID == id {
			return toTask(r, sn.agg[id]), true
		}
	}
	return Task{}, false
}

// get — одно дело с вычисляемыми полями (после создания/правки).
func get(ctx context.Context, q *store.Queries, id int64) (Task, error) {
	sn, err := loadSnapshot(ctx, q)
	if err != nil {
		return Task{}, err
	}
	t, ok := sn.task(id)
	if !ok {
		return Task{}, errTaskNotFound
	}
	return t, nil
}

func fieldsOf(t store.Task) Fields {
	return Fields{Title: t.Title, Blocks: int(t.Blocks), Importance: int(t.Importance), Deadline: t.Deadline, TypeID: t.TypeID, Note: t.Note}
}

// ---------- типы ----------

func (s *Service) Types(ctx context.Context) ([]Type, error) {
	rows, err := store.New(s.pool).ListTypes(ctx)
	if err != nil {
		return nil, fmt.Errorf("task types: %w", err)
	}
	out := make([]Type, len(rows))
	for i, r := range rows {
		out[i] = toType(r)
	}
	return out, nil
}

func (s *Service) CreateType(ctx context.Context, in TypeInput) (Type, error) {
	in, err := in.Normalize()
	if err != nil {
		return Type{}, err
	}
	var t store.TaskType
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		pos, err := q.NextTypePosition(ctx)
		if err != nil {
			return fmt.Errorf("position: %w", err)
		}
		t, err = q.CreateType(ctx, store.CreateTypeParams{Name: in.Name, Color: in.Color, Position: pos, CreatedAt: s.clock.Now()})
		return err
	})
	if db.IsUniqueViolation(err) {
		return Type{}, errDuplicateType
	}
	if err != nil {
		return Type{}, fmt.Errorf("task type create: %w", err)
	}
	return toType(t), nil
}

func (s *Service) UpdateType(ctx context.Context, id int64, p TypePatch) (Type, error) {
	var t store.TaskType
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		cur, err := q.GetType(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return errTypeNotFound
		}
		if err != nil {
			return fmt.Errorf("get: %w", err)
		}
		in, err := p.Apply(TypeInput{Name: cur.Name, Color: cur.Color})
		if err != nil {
			return err
		}
		t, err = q.UpdateType(ctx, store.UpdateTypeParams{ID: id, Name: in.Name, Color: in.Color})
		return err
	})
	if db.IsUniqueViolation(err) {
		return Type{}, errDuplicateType
	}
	if err != nil {
		return Type{}, fmt.Errorf("task type update %d: %w", id, err)
	}
	return toType(t), nil
}

// DeleteType удаляет тип; дела этого типа остаются без типа.
func (s *Service) DeleteType(ctx context.Context, id int64) error {
	n, err := store.New(s.pool).DeleteType(ctx, id)
	if err != nil {
		return fmt.Errorf("task type delete %d: %w", id, err)
	}
	if n == 0 {
		return errTypeNotFound
	}
	return nil
}

func (s *Service) ReorderTypes(ctx context.Context, ids []int64) error {
	return s.reorder(ctx, ids,
		func(q *store.Queries) ([]int64, error) {
			rows, err := q.ListTypes(ctx)
			out := make([]int64, len(rows))
			for i, r := range rows {
				out[i] = r.ID
			}
			return out, err
		},
		func(q *store.Queries, id int64, pos int32) error {
			return q.SetTypePosition(ctx, store.SetTypePositionParams{ID: id, Position: pos})
		})
}

// reorder — общий «PUT /order» для типов и колонок: ids — полный список в новом порядке.
func (s *Service) reorder(ctx context.Context, ids []int64,
	existing func(*store.Queries) ([]int64, error), set func(*store.Queries, int64, int32) error,
) error {
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		cur, err := existing(q)
		if err != nil {
			return fmt.Errorf("list: %w", err)
		}
		if err := CheckOrder(ids, cur); err != nil {
			return err
		}
		for i, id := range ids {
			if err := set(q, id, int32(i)); err != nil {
				return fmt.Errorf("set position: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("reorder: %w", err)
	}
	return nil
}

// ---------- колонки ----------

func (s *Service) Columns(ctx context.Context) ([]Column, error) {
	rows, err := store.New(s.pool).ListColumns(ctx)
	if err != nil {
		return nil, fmt.Errorf("columns: %w", err)
	}
	out := make([]Column, len(rows))
	for i, r := range rows {
		out[i] = toColumn(r)
	}
	return out, nil
}

func (s *Service) CreateColumn(ctx context.Context, rawName string) (Column, error) {
	name, err := ColumnName(rawName)
	if err != nil {
		return Column{}, err
	}
	var c store.TaskColumn
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		pos, err := q.NextColumnPosition(ctx)
		if err != nil {
			return fmt.Errorf("position: %w", err)
		}
		c, err = q.CreateColumn(ctx, store.CreateColumnParams{Name: name, Position: pos, CreatedAt: s.clock.Now()})
		return err
	})
	if err != nil {
		return Column{}, fmt.Errorf("column create: %w", err)
	}
	return toColumn(c), nil
}

func (s *Service) RenameColumn(ctx context.Context, id int64, rawName string) (Column, error) {
	name, err := ColumnName(rawName)
	if err != nil {
		return Column{}, err
	}
	c, err := store.New(s.pool).RenameColumn(ctx, store.RenameColumnParams{ID: id, Name: name})
	if errors.Is(err, pgx.ErrNoRows) {
		return Column{}, errColumnNotFound
	}
	if err != nil {
		return Column{}, fmt.Errorf("column rename %d: %w", id, err)
	}
	return toColumn(c), nil
}

// DeleteColumn удаляет колонку. Если в ней есть дела (и выполненные тоже), нужно сказать,
// куда их перенести (to): они встанут в конец той колонки в своём порядке. Иначе 409 column_not_empty.
func (s *Service) DeleteColumn(ctx context.Context, id int64, to *int64) error {
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if _, err := q.GetColumnForUpdate(ctx, id); errors.Is(err, pgx.ErrNoRows) {
			return errColumnNotFound
		} else if err != nil {
			return fmt.Errorf("get: %w", err)
		}
		n, err := q.CountColumnTasks(ctx, id)
		if err != nil {
			return fmt.Errorf("count: %w", err)
		}
		if n > 0 {
			if to == nil {
				return httpx.Conflict("column_not_empty", "column has tasks: pass ?to=<column id> to move them")
			}
			if *to == id {
				return errInvalidColumn
			}
			if err := lockColumn(ctx, q, *to); err != nil {
				return err
			}
			offset, err := q.NextTaskPosition(ctx, *to)
			if err != nil {
				return fmt.Errorf("position: %w", err)
			}
			if err := q.MoveAllTasks(ctx, store.MoveAllTasksParams{FromColumn: id, ToColumn: *to, OffsetBy: offset}); err != nil {
				return fmt.Errorf("move tasks: %w", err)
			}
		}
		_, err = q.DeleteColumn(ctx, id)
		return err
	})
	if err != nil {
		return fmt.Errorf("column delete %d: %w", id, err)
	}
	return nil
}

func (s *Service) ReorderColumns(ctx context.Context, ids []int64) error {
	return s.reorder(ctx, ids,
		func(q *store.Queries) ([]int64, error) {
			rows, err := q.ListColumns(ctx)
			out := make([]int64, len(rows))
			for i, r := range rows {
				out[i] = r.ID
			}
			return out, err
		},
		func(q *store.Queries, id int64, pos int32) error {
			return q.SetColumnPosition(ctx, store.SetColumnPositionParams{ID: id, Position: pos})
		})
}

// lockColumn — колонка из тела запроса существует (и заблокирована до конца транзакции).
func lockColumn(ctx context.Context, q *store.Queries, id int64) error {
	_, err := q.GetColumnForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return errInvalidColumn
	}
	if err != nil {
		return fmt.Errorf("lock column: %w", err)
	}
	return nil
}

// ---------- дела ----------

// Filter — какие дела отдавать: невыполненные (по умолчанию), выполненные или все.
type Filter string

const (
	FilterOpen Filter = "false"
	FilterDone Filter = "true"
	FilterAll  Filter = "all"
)

// Tasks — дела с вычисленными полями. Родители тоже в списке (доска показывает листья,
// а хлебные крошки «Родитель › шаг» берутся отсюда же). Фильтр — по вычисленной выполненности.
func (s *Service) Tasks(ctx context.Context, f Filter) ([]Task, error) {
	if f != FilterOpen && f != FilterDone && f != FilterAll {
		return nil, httpx.BadRequest("invalid_filter", "done must be true, false or all")
	}
	sn, err := loadSnapshot(ctx, store.New(s.pool))
	if err != nil {
		return nil, fmt.Errorf("tasks: %w", err)
	}
	out := make([]Task, 0, len(sn.rows))
	for _, r := range sn.rows {
		t := toTask(r, sn.agg[r.ID])
		if f == FilterAll || t.Done == (f == FilterDone) {
			out = append(out, t)
		}
	}
	return out, nil
}

// Tree — всё дерево, в которое входит дело (от корня), в порядке обхода в глубину:
// для графа дела (§18.3, B2). Соседи — по порядку в колонке.
func (s *Service) Tree(ctx context.Context, id int64) ([]Task, error) {
	sn, err := loadSnapshot(ctx, store.New(s.pool))
	if err != nil {
		return nil, fmt.Errorf("task tree %d: %w", id, err)
	}
	a, ok := sn.agg[id]
	if !ok {
		return nil, errTaskNotFound
	}
	children := make(map[int64][]store.Task)
	byID := make(map[int64]store.Task, len(sn.rows))
	for _, r := range sn.rows { // rows уже по (column_id, position, id)
		byID[r.ID] = r
		if r.ParentID != nil {
			children[*r.ParentID] = append(children[*r.ParentID], r)
		}
	}
	var out []Task
	var walk func(r store.Task)
	walk = func(r store.Task) {
		out = append(out, toTask(r, sn.agg[r.ID]))
		for _, c := range children[r.ID] {
			walk(c)
		}
	}
	walk(byID[a.Root])
	return out, nil
}

// Split добавляет делу шаги (§18.3, B2). Шаги встают в колонку родителя сразу после него;
// важность, дедлайн и тип по умолчанию наследуются. Возвращает всё дерево (как Tree).
func (s *Service) Split(ctx context.Context, id int64, in SplitInput) ([]Task, error) {
	if err := in.checkCount(); err != nil {
		return nil, err
	}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		parent, err := q.GetTaskForUpdate(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return errTaskNotFound
		}
		if err != nil {
			return fmt.Errorf("get: %w", err)
		}
		active, err := q.HasActiveFocus(ctx, id)
		if err != nil {
			return fmt.Errorf("active focus: %w", err)
		}
		if active {
			return httpx.Conflict("focus_active", "the task has an active focus session: finish it first")
		}
		sn, err := loadSnapshot(ctx, q)
		if err != nil {
			return err
		}
		if err := CheckSplit(sn.agg[id]); err != nil {
			return err
		}
		steps, err := in.Normalize(fieldsOf(parent))
		if err != nil {
			return err
		}
		now := s.clock.Now()
		ids := make([]int64, len(steps))
		for i, f := range steps {
			if err := checkType(ctx, q, f.TypeID); err != nil {
				return err
			}
			t, err := q.CreateTask(ctx, store.CreateTaskParams{
				ParentID: &id, Title: f.Title, Blocks: int16(f.Blocks), Importance: int16(f.Importance), Deadline: f.Deadline,
				TypeID: f.TypeID, ColumnID: parent.ColumnID, Note: f.Note, Now: now, // position — ниже, общей раскладкой
			})
			if err != nil {
				return fmt.Errorf("create step: %w", err)
			}
			ids[i] = t.ID
		}
		// Разбитое дело больше не планируется — в плане теперь место его шагам (§18.3, B3).
		if err := q.DeleteTaskPlans(ctx, id); err != nil {
			return fmt.Errorf("delete plans: %w", err)
		}
		column, err := q.ColumnTaskIDs(ctx, parent.ColumnID)
		if err != nil {
			return fmt.Errorf("column tasks: %w", err)
		}
		column = slices.DeleteFunc(column, func(x int64) bool { return slices.Contains(ids, x) })
		return q.PlaceTasks(ctx, store.PlaceTasksParams{ColumnID: parent.ColumnID, Ids: PlaceAfter(column, id, ids)})
	})
	if err != nil {
		return nil, fmt.Errorf("task split %d: %w", id, err)
	}
	return s.Tree(ctx, id)
}

// Create добавляет дело в конец колонки.
func (s *Service) Create(ctx context.Context, in TaskInput) (Task, error) {
	f, err := in.Normalize()
	if err != nil {
		return Task{}, err
	}
	var t Task
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := lockColumn(ctx, q, in.ColumnID); err != nil {
			return err
		}
		if err := checkType(ctx, q, f.TypeID); err != nil {
			return err
		}
		pos, err := q.NextTaskPosition(ctx, in.ColumnID)
		if err != nil {
			return fmt.Errorf("position: %w", err)
		}
		created, err := q.CreateTask(ctx, store.CreateTaskParams{
			Title: f.Title, Blocks: int16(f.Blocks), Importance: int16(f.Importance), Deadline: f.Deadline,
			TypeID: f.TypeID, ColumnID: in.ColumnID, Position: pos, Note: f.Note, Now: s.clock.Now(),
		})
		if err != nil {
			return err
		}
		t = toTask(created, Agg{Leaf: true, Blocks: f.Blocks}) // новое дело — всегда лист без шагов
		return nil
	})
	if err != nil {
		return Task{}, fmt.Errorf("task create: %w", err)
	}
	return t, nil
}

func (s *Service) Update(ctx context.Context, id int64, p Patch) (Task, error) {
	var t Task
	completed := false // лист стал выполненным именно сейчас — после коммита публикуем TaskCompleted
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		cur, err := q.GetTask(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return errTaskNotFound
		}
		if err != nil {
			return fmt.Errorf("get: %w", err)
		}
		now := s.clock.Now()
		f, doneAt, err := p.Apply(fieldsOf(cur), cur.DoneAt, now)
		if err != nil {
			return err
		}
		if p.TypeID.Set {
			if err := checkType(ctx, q, f.TypeID); err != nil {
				return err
			}
		}
		if p.Done != nil {
			// Родитель выполнен, когда выполнены все шаги — отметить его вручную нельзя.
			hasKids, err := q.HasChildren(ctx, id)
			if err != nil {
				return fmt.Errorf("has children: %w", err)
			}
			if hasKids {
				return httpx.Conflict("task_not_leaf", "a task with steps is done when all its steps are done")
			}
		}
		if _, err := q.UpdateTask(ctx, store.UpdateTaskParams{
			ID: id, Title: f.Title, Blocks: int16(f.Blocks), Importance: int16(f.Importance), Deadline: f.Deadline,
			TypeID: f.TypeID, Note: f.Note, DoneAt: doneAt, Now: now,
		}); err != nil {
			return err
		}
		completed = cur.DoneAt == nil && doneAt != nil
		t, err = get(ctx, q, id)
		return err
	})
	if err != nil {
		return Task{}, fmt.Errorf("task update %d: %w", id, err)
	}
	if completed {
		s.bus.Publish(eventbus.TaskCompleted{TaskID: id, At: *t.DoneAt})
	}
	return t, nil
}

// MoveInput — тело POST /api/tasks/{id}/move: в какую колонку и перед каким делом (null — в конец).
// beforeId, а не индекс: на доске могут быть скрыты выполненные дела, и индекс фронта
// не совпал бы с порядком в БД.
type MoveInput struct {
	ColumnID int64  `json:"columnId"`
	BeforeID *int64 `json:"beforeId"`
}

func (s *Service) Move(ctx context.Context, id int64, in MoveInput) error {
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if _, err := q.GetTask(ctx, id); errors.Is(err, pgx.ErrNoRows) {
			return errTaskNotFound
		} else if err != nil {
			return fmt.Errorf("get: %w", err)
		}
		if err := lockColumn(ctx, q, in.ColumnID); err != nil {
			return err
		}
		ids, err := q.ColumnTaskIDs(ctx, in.ColumnID)
		if err != nil {
			return fmt.Errorf("column tasks: %w", err)
		}
		ids, err = Reorder(ids, id, in.BeforeID)
		if err != nil {
			return err
		}
		// Перенумеровываем всю целевую колонку; в исходной остаётся дырка в position — порядок от неё не страдает.
		return q.PlaceTasks(ctx, store.PlaceTasksParams{ColumnID: in.ColumnID, Ids: ids})
	})
	if err != nil {
		return fmt.Errorf("task move %d: %w", id, err)
	}
	return nil
}

// Get — одно дело с вычисленными полями (для focus: что за дело, лист ли, выполнено ли).
func (s *Service) Get(ctx context.Context, id int64) (Task, error) {
	return get(ctx, store.New(s.pool), id)
}

// Complete отмечает дело выполненным («Я закончил» в фокусе). Уже выполненное — без изменений
// (Patch.Apply не сдвигает дату), дело с шагами — 409 task_not_leaf, как и через PATCH.
func (s *Service) Complete(ctx context.Context, id int64) error {
	_, err := s.Update(ctx, id, Patch{Done: new(true)})
	return err
}

// Delete удаляет дело вместе со всеми шагами (ON DELETE CASCADE).
func (s *Service) Delete(ctx context.Context, id int64) error {
	n, err := store.New(s.pool).DeleteTask(ctx, id)
	if err != nil {
		return fmt.Errorf("task delete %d: %w", id, err)
	}
	if n == 0 {
		return errTaskNotFound
	}
	return nil
}

func checkType(ctx context.Context, q *store.Queries, id *int64) error {
	if id == nil {
		return nil
	}
	ok, err := q.TypeExists(ctx, *id)
	if err != nil {
		return fmt.Errorf("type exists: %w", err)
	}
	if !ok {
		return errInvalidType
	}
	return nil
}
