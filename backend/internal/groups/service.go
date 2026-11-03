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
type Group struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	Color         *string   `json:"color"`
	CreatedAt     time.Time `json:"createdAt"`
	WordCount     int       `json:"wordCount"`
	Due           int       `json:"due"` // карточки на сегодня, включая новые (§6)
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
			CreatedAt:     r.CreatedAt,
			WordCount:     int(r.Words),
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

// Get — группа + аналитика (§5) + её слова.
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

	ws, err := s.words.List(ctx, words.ListFilter{GroupID: &id})
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

// UpdateParams — тело PATCH /api/groups/:id.
type UpdateParams struct {
	Name  *string        `json:"name"`
	Color OptionalString `json:"color"` // "#RRGGBB" или null
}

var colorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func (s *Service) Update(ctx context.Context, id int64, p UpdateParams) (Group, error) {
	var name *string
	if p.Name != nil {
		n, ok := words.NormalizeGroupName(*p.Name)
		if !ok {
			return Group{}, httpx.BadRequest("invalid_group_name", "group name must be a single tag")
		}
		name = &n
	}
	var color *string
	if p.Color.Value != nil {
		if !colorRe.MatchString(*p.Color.Value) {
			return Group{}, httpx.BadRequest("invalid_color", "color must be #RRGGBB")
		}
		c := strings.ToLower(*p.Color.Value)
		color = &c
	}

	var g Group
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		cur, err := q.GetGroup(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("group not found")
		}
		if err != nil {
			return fmt.Errorf("get group: %w", err)
		}
		params := store.UpdateGroupParams{ID: id, Name: cur.Name, Color: cur.Color}
		if name != nil {
			params.Name = *name
		}
		if p.Color.Set {
			params.Color = color // nil при null — очистить
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

// Delete удаляет группу. Слова остаются (связи — каскадом), история сессий тоже (group_id → NULL).
func (s *Service) Delete(ctx context.Context, id int64) error {
	n, err := store.New(s.pool).DeleteGroup(ctx, id)
	if err != nil {
		return fmt.Errorf("groups delete: %w", err)
	}
	if n == 0 {
		return httpx.NotFound("group not found")
	}
	return nil
}
