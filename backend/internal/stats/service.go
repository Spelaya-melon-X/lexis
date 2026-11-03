package stats

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"lexis/internal/platform/clock"
	"lexis/internal/platform/httpx"
	"lexis/internal/stats/store"
)

// Service — только чтение: всё считается на лету из training_sessions/reviews/words.
// Подписка на eventbus stats не нужна — хранить нечего, агрегаты дешёвые.
type Service struct {
	pool  *pgxpool.Pool
	clock clock.Clock
}

func NewService(pool *pgxpool.Pool, clk clock.Clock) *Service {
	return &Service{pool: pool, clock: clk}
}

type GroupWords struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Words int    `json:"words"` // уникальных слов за день
}

type Totals struct {
	Words   int    `json:"words"`
	Learned int    `json:"learned"` // уровень ≥ 4
	Levels  [5]int `json:"levels"`  // [ур.1, …, ур.5]
}

type Overview struct {
	Day           DayStats     `json:"day"`
	GroupsTrained int          `json:"groupsTrained"`
	ByGroup       []GroupWords `json:"byGroup"`
	Streak        Streak       `json:"streak"` // относительно сегодня, не выбранного дня
	Totals        Totals       `json:"totals"`
}

// Overview — сводка за день (YYYY-MM-DD, пусто — сегодня) + серия и общие итоги.
func (s *Service) Overview(ctx context.Context, day string) (Overview, error) {
	now := s.clock.Now()
	d := now
	if day != "" {
		var err error
		if d, err = s.parseDay(day); err != nil {
			return Overview{}, err
		}
	}
	from, to := clock.DayBounds(d)
	q := store.New(s.pool)

	rows, err := s.sessions(ctx, q, from, to)
	if err != nil {
		return Overview{}, fmt.Errorf("stats overview: %w", err)
	}
	byGroup, err := q.WordsByGroup(ctx, store.WordsByGroupParams{FromAt: from, ToAt: to})
	if err != nil {
		return Overview{}, fmt.Errorf("stats overview: words by group: %w", err)
	}
	finished, err := q.ListAllFinishedAt(ctx)
	if err != nil {
		return Overview{}, fmt.Errorf("stats overview: finished: %w", err)
	}
	lv, err := q.LevelTotals(ctx)
	if err != nil {
		return Overview{}, fmt.Errorf("stats overview: levels: %w", err)
	}

	o := Overview{
		Day:           BucketByDay(rows, from, 1)[0],
		GroupsTrained: GroupsTrained(rows),
		ByGroup:       make([]GroupWords, len(byGroup)),
		Streak:        Streaks(finished, now),
	}
	for i, g := range byGroup {
		o.ByGroup[i] = GroupWords{ID: g.ID, Name: g.Name, Words: int(g.Words)}
	}
	o.Totals.Levels = [5]int{int(lv.Level1), int(lv.Level2), int(lv.Level3), int(lv.Level4), int(lv.Level5)}
	for i, n := range o.Totals.Levels {
		o.Totals.Words += n
		if i >= 3 {
			o.Totals.Learned += n
		}
	}
	return o, nil
}

// MaxCalendarDays — потолок окна календаря: год + 1 день (високосный).
const MaxCalendarDays = 366

type Calendar struct {
	From string     `json:"from"`
	To   string     `json:"to"`
	Days []DayStats `json:"days"` // каждый день окна, включая пустые
}

// Calendar — heatmap за [from, to] включительно (YYYY-MM-DD). Пусто: to — сегодня, from — to − 364 дня.
func (s *Service) Calendar(ctx context.Context, fromS, toS string) (Calendar, error) {
	to := clock.StartOfDay(s.clock.Now())
	if toS != "" {
		var err error
		if to, err = s.parseDay(toS); err != nil {
			return Calendar{}, err
		}
	}
	from := to.AddDate(0, 0, -364)
	if fromS != "" {
		var err error
		if from, err = s.parseDay(fromS); err != nil {
			return Calendar{}, err
		}
	}
	days := clock.DaysBetween(from, to) + 1
	if days < 1 || days > MaxCalendarDays {
		return Calendar{}, httpx.BadRequest("invalid_range", fmt.Sprintf("from must be ≤ to, at most %d days", MaxCalendarDays))
	}

	_, end := clock.DayBounds(to)
	rows, err := s.sessions(ctx, store.New(s.pool), from, end)
	if err != nil {
		return Calendar{}, fmt.Errorf("stats calendar: %w", err)
	}
	return Calendar{
		From: from.Format(time.DateOnly),
		To:   to.Format(time.DateOnly),
		Days: BucketByDay(rows, from, days),
	}, nil
}

type Range string

const (
	RangeWeek  Range = "week"
	RangeMonth Range = "month"
)

type Trends struct {
	Range Range      `json:"range"`
	Days  []DayStats `json:"days"`
}

// Trends — последние 7 (week) или 30 (month) дней, включая сегодня.
func (s *Service) Trends(ctx context.Context, r Range) (Trends, error) {
	var days int
	switch r {
	case RangeWeek:
		days = 7
	case RangeMonth:
		days = 30
	default:
		return Trends{}, httpx.BadRequest("invalid_range", "range must be week or month")
	}
	now := s.clock.Now()
	from := clock.StartOfDay(now).AddDate(0, 0, -(days - 1))
	_, end := clock.DayBounds(now)
	rows, err := s.sessions(ctx, store.New(s.pool), from, end)
	if err != nil {
		return Trends{}, fmt.Errorf("stats trends: %w", err)
	}
	return Trends{Range: r, Days: BucketByDay(rows, from, days)}, nil
}

func (s *Service) sessions(ctx context.Context, q *store.Queries, from, to time.Time) ([]SessionRow, error) {
	rows, err := q.ListFinishedSessions(ctx, store.ListFinishedSessionsParams{FromAt: from, ToAt: to})
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	out := make([]SessionRow, len(rows))
	for i, r := range rows {
		out[i] = SessionRow{
			FinishedAt: r.FinishedAt, GroupID: r.GroupID, Total: int(r.Total), Good: int(r.Good),
			Hard: int(r.Hard), Again: int(r.Again), DurationSec: int(r.DurationSec),
		}
	}
	return out, nil
}

// parseDay: "2026-11-02" → полночь этого дня в TZ пользователя.
func (s *Service) parseDay(v string) (time.Time, error) {
	t, err := time.ParseInLocation(time.DateOnly, v, s.clock.Location())
	if err != nil {
		return time.Time{}, httpx.BadRequest("invalid_date", "date must be YYYY-MM-DD")
	}
	return t, nil
}
