package groups

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lexis/internal/groups/store"
	"lexis/internal/groups/tree"
	"lexis/internal/platform/clock"
	"lexis/internal/platform/db"
	"lexis/internal/platform/httpx"
	"lexis/internal/words"
)

// WordLister — то, что groups нужно от модуля words: список слов группы для GET /:id.
// Узкий интерфейс вместо *words.Service — groups не зависит от остального API words,
// а в тестах его легко подменить.
type WordLister interface {
	List(ctx context.Context, f words.ListFilter) ([]words.Word, error)
}

type Service struct {
	pool  *pgxpool.Pool
	clock clock.Clock
	words WordLister
}

func NewService(pool *pgxpool.Pool, clk clock.Clock, wl WordLister) *Service {
	return &Service{pool: pool, clock: clk, words: wl}
}

// Group — группа в списке: статус дня, due и распределение по уровням.
// Счётчики — по поддереву (сама группа + подгруппы, слова уникально, §18.2).
type Group struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	Color         *string   `json:"color"`
	ParentID      *int64    `json:"parentId"` // null — корень
	CreatedAt     time.Time `json:"createdAt"`
	WordCount     int       `json:"wordCount"`
	OwnWordCount  int       `json:"ownWordCount"` // слова, привязанные к самой группе
	Due           int       `json:"due"`          // карточки на сегодня, включая новые (§6)
	RepeatedToday bool      `json:"repeatedToday"`
	Levels        Levels    `json:"levels"` // [ур.1, …, ур.5]
}

type Analytics struct {
	MasteredPercent   int        `json:"masteredPercent"`
	SessionsThisWeek  int        `json:"sessionsThisWeek"`  // с понедельника, включая free
	SessionsThisMonth int        `json:"sessionsThisMonth"` // с 1-го числа, включая free
	Range             Range      `json:"range"`
	Productivity      []DayPoint `json:"productivity"`
}

type Detail struct {
	Group     Group        `json:"group"`
	Analytics Analytics    `json:"analytics"`
	Words     []words.Word `json:"words"`
}

func (s *Service) List(ctx context.Context) ([]Group, error) {
	groups, err := s.summaries(ctx, store.New(s.pool), nil)
	if err != nil {
		return nil, fmt.Errorf("groups list: %w", err)
	}
	return groups, nil
}

func (s *Service) summaries(ctx context.Context, q *store.Queries, id *int64) ([]Group, error) {
	dayStart, dayEnd := clock.DayBounds(s.clock.Now())
	rows, err := q.ListGroupSummaries(ctx, store.ListGroupSummariesParams{DayStart: dayStart, DayEnd: dayEnd, ID: id})
	if err != nil {
		return nil, fmt.Errorf("group summaries: %w", err)
	}
	groups := make([]Group, len(rows))
	for i, r := range rows {
		groups[i] = Group{
			ID:            r.ID,
			Name:          r.Name,
			Color:         r.Color,
			ParentID:      r.ParentID,
			CreatedAt:     r.CreatedAt,
			WordCount:     int(r.Words),
			OwnWordCount:  int(r.OwnWords),
			Due:           int(r.Due),
			RepeatedToday: RepeatedToday(r.TrainedToday, int(r.Due)),
			Levels:        Levels{int(r.Level1), int(r.Level2), int(r.Level3), int(r.Level4), int(r.Level5)},
		}
	}
	return groups, nil
}

func (s *Service) summary(ctx context.Context, q *store.Queries, id int64) (Group, error) {
	groups, err := s.summaries(ctx, q, &id)
	if err != nil {
		return Group{}, err
	}
	if len(groups) == 0 {
		return Group{}, httpx.NotFound("group not found")
	}
	return groups[0], nil
}

// Get — группа + аналитика (§5) + слова, всё по поддереву.
func (s *Service) Get(ctx context.Context, id int64, r Range) (Detail, error) {
	days, ok := r.Days()
	if !ok {
		return Detail{}, httpx.BadRequest("invalid_range", "range must be week or month")
	}
	q := store.New(s.pool)
	g, err := s.summary(ctx, q, id)
	if err != nil {
		return Detail{}, fmt.Errorf("groups get: %w", err)
	}

	now := s.clock.Now()
	_, dayEnd := clock.DayBounds(now)
	week, err := q.CountFinishedSessions(ctx, store.CountFinishedSessionsParams{GroupID: id, FromAt: WeekStart(now), ToAt: dayEnd})
	if err != nil {
		return Detail{}, fmt.Errorf("groups get: week sessions: %w", err)
	}
	month, err := q.CountFinishedSessions(ctx, store.CountFinishedSessionsParams{GroupID: id, FromAt: MonthStart(now), ToAt: dayEnd})
	if err != nil {
		return Detail{}, fmt.Errorf("groups get: month sessions: %w", err)
	}

	from := clock.StartOfDay(now).AddDate(0, 0, -(days - 1))
	rows, err := q.ListFinishedSessions(ctx, store.ListFinishedSessionsParams{GroupID: id, FromAt: from, ToAt: dayEnd})
	if err != nil {
		return Detail{}, fmt.Errorf("groups get: sessions: %w", err)
	}
	stats := make([]SessionStat, len(rows))
	for i, row := range rows {
		stats[i] = SessionStat{FinishedAt: row.FinishedAt, Total: int(row.Total), Good: int(row.Good)}
	}

	ws, err := s.words.List(ctx, words.ListFilter{GroupID: &id, Subtree: true})
	if err != nil {
		return Detail{}, fmt.Errorf("groups get: %w", err)
	}

	return Detail{
		Group: g,
		Analytics: Analytics{
			MasteredPercent:   g.Levels.MasteredPercent(),
			SessionsThisWeek:  int(week),
			SessionsThisMonth: int(month),
			Range:             r,
			Productivity:      Productivity(stats, now, days),
		},
		Words: ws,
	}, nil
}

// OptionalString — поле PATCH, у которого «нет в запросе» и null означают разное:
// нет — не менять, null — очистить. Обычный *string их не различает.
// encoding/json вызывает UnmarshalJSON и для null, но не для отсутствующего поля.
type OptionalString struct {
	Set   bool
	Value *string
}

func (o *OptionalString) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		o.Value = nil
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	o.Value = &s
	return nil
}

// CreateParams — тело POST /api/groups: пустая группа (папка), в т.ч. сразу внутри другой.
type CreateParams struct {
	Name     string  `json:"name"`
	ParentID *int64  `json:"parentId"`
	Color    *string `json:"color"`
}

func (s *Service) Create(ctx context.Context, p CreateParams) (Group, error) {
	name, err := normalizeName(p.Name)
	if err != nil {
		return Group{}, err
	}
	color, err := normalizeColor(p.Color)
	if err != nil {
		return Group{}, err
	}
	var g Group
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		t, err := loadTree(ctx, q)
		if err != nil {
			return err
		}
		parent, err := parentOf(p.ParentID)
		if err != nil {
			return err
		}
		if err := treeError(t.CheckNew(parent)); err != nil {
			return err
		}
		id, err := q.InsertGroup(ctx, store.InsertGroupParams{Name: name, ParentID: p.ParentID, Color: color, Now: s.clock.Now()})
		if err != nil {
			if db.IsUniqueViolation(err) {
				return httpx.Conflict("duplicate_group", "group with this name already exists")
			}
			return fmt.Errorf("insert group: %w", err)
		}
		g, err = s.summary(ctx, q, id)
		return err
	})
	if err != nil {
		return Group{}, fmt.Errorf("groups create: %w", err)
	}
	return g, nil
}

// UpdateParams — тело PATCH /api/groups/:id.
type UpdateParams struct {
	Name     *string               `json:"name"`
	Color    OptionalString        `json:"color"`    // "#RRGGBB" или null
	ParentID httpx.Optional[int64] `json:"parentId"` // id или null — в корень
}

var colorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func (s *Service) Update(ctx context.Context, id int64, p UpdateParams) (Group, error) {
	var name *string
	if p.Name != nil {
		n, err := normalizeName(*p.Name)
		if err != nil {
			return Group{}, err
		}
		name = &n
	}
	color, err := normalizeColor(p.Color.Value)
	if err != nil {
		return Group{}, err
	}

	var g Group
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		// Блокируем дерево при любой правке: переименование тоже может столкнуться
		// с созданием группы по пути из тега в параллельном bulk.
		t, err := loadTree(ctx, q)
		if err != nil {
			return err
		}
		cur, err := q.GetGroup(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("group not found")
		}
		if err != nil {
			return fmt.Errorf("get group: %w", err)
		}
		params := store.UpdateGroupParams{ID: id, Name: cur.Name, Color: cur.Color, ParentID: cur.ParentID}
		if name != nil {
			params.Name = *name
		}
		if p.Color.Set {
			params.Color = color // nil при null — очистить
		}
		if p.ParentID.Set {
			parent, err := parentOf(p.ParentID.Value)
			if err != nil {
				return err
			}
			if err := treeError(t.CheckMove(id, parent)); err != nil {
				return err
			}
			params.ParentID = p.ParentID.Value
		}
		if _, err := q.UpdateGroup(ctx, params); err != nil {
			if db.IsUniqueViolation(err) {
				return httpx.Conflict("duplicate_group", "group with this name already exists")
			}
			return fmt.Errorf("update group: %w", err)
		}
		g, err = s.summary(ctx, q, id)
		return err
	})
	if err != nil {
		return Group{}, fmt.Errorf("groups update: %w", err)
	}
	return g, nil
}

// Children — что делать с подгруппами при удалении (§18.2).
type Children string

const (
	ChildrenLift   Children = "lift"   // поднять на уровень удаляемой группы
	ChildrenDelete Children = "delete" // удалить всё поддерево
)

// Delete удаляет группу (и, если просили, подгруппы). Слова остаются (связи — каскадом),
// история сессий тоже (group_id → NULL).
func (s *Service) Delete(ctx context.Context, id int64, children Children) error {
	if children != ChildrenLift && children != ChildrenDelete {
		return httpx.BadRequest("invalid_children", "children must be lift or delete")
	}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		t, err := loadTree(ctx, q)
		if err != nil {
			return err
		}
		if !t.Has(id) {
			return httpx.NotFound("group not found")
		}
		ids := t.Subtree(id)
		if children == ChildrenLift {
			// Подъём только уменьшает глубину — проверять дерево не нужно.
			if err := q.LiftChildren(ctx, store.LiftChildrenParams{ID: id, NewParent: ptrOrNil(t.Parent(id))}); err != nil {
				return fmt.Errorf("lift children: %w", err)
			}
			ids = []int64{id}
		}
		if _, err := q.DeleteGroups(ctx, ids); err != nil {
			return fmt.Errorf("delete groups: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("groups delete: %w", err)
	}
	return nil
}

// loadTree блокирует дерево до конца транзакции и читает его целиком (групп — десятки).
func loadTree(ctx context.Context, q *store.Queries) (*tree.Tree, error) {
	if err := q.LockTree(ctx, tree.LockKey); err != nil {
		return nil, fmt.Errorf("lock group tree: %w", err)
	}
	rows, err := q.ListGroupNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list group nodes: %w", err)
	}
	nodes := make([]tree.Node, len(rows))
	for i, r := range rows {
		nodes[i] = tree.Node{ID: r.ID, Name: r.Name, Parent: deref(r.ParentID)}
	}
	return tree.New(nodes), nil
}

// treeError переводит ошибки проверок дерева в ответы API.
func treeError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, tree.ErrNotFound):
		return httpx.BadRequest("invalid_parent", "parent group not found")
	case errors.Is(err, tree.ErrCycle):
		return httpx.BadRequest("group_cycle", "group cannot be moved into itself or its subgroup")
	case errors.Is(err, tree.ErrTooDeep):
		return httpx.BadRequest("group_too_deep", fmt.Sprintf("groups can be nested at most %d levels deep", tree.MaxDepth))
	}
	return err
}

func normalizeName(raw string) (string, error) {
	n, ok := words.NormalizeGroupName(raw)
	if !ok {
		return "", httpx.BadRequest("invalid_group_name", "group name must be a single tag")
	}
	return n, nil
}

// normalizeColor: nil — без цвета; иначе "#rrggbb" в нижнем регистре.
func normalizeColor(c *string) (*string, error) {
	if c == nil {
		return nil, nil
	}
	if !colorRe.MatchString(*c) {
		return nil, httpx.BadRequest("invalid_color", "color must be #RRGGBB")
	}
	lc := strings.ToLower(*c)
	return &lc, nil
}

// parentOf: nil — корень (0); id ≤ 0 — ошибка, а не «корень», чтобы {"parentId": 0} не уехал туда молча.
func parentOf(p *int64) (int64, error) {
	if p != nil && *p <= 0 {
		return 0, httpx.BadRequest("invalid_parent", "parentId must be a positive id or null")
	}
	return deref(p), nil
}

// deref / ptrOrNil — мост между parent_id (NULL = корень) и tree (0 = корень).
func deref(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

func ptrOrNil(id int64) *int64 {
	if id == 0 {
		return nil
	}
	return &id
}
