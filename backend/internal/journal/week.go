package journal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"lexis/internal/habits"
	"lexis/internal/journal/store"
	"lexis/internal/platform/clock"
	"lexis/internal/platform/days"
	"lexis/internal/platform/httpx"
)

// Недельный анализ (SPEC §17.3).

const (
	MaxBlocks        = 30
	MaxTopicLen      = 200
	MaxConclusionLen = 20000
)

// HabitSource — то, что сводке недели нужно от привычек (реализует *habits.Service).
// Узкий интерфейс, как groups.WordLister: journal не тянет модуль habits целиком.
type HabitSource interface {
	PeriodStats(ctx context.Context, from, to time.Time) ([]habits.PeriodStat, error)
	Goals(ctx context.Context, fromS, toS string) ([]habits.Goal, error)
}

// Block — «О чём» + «Вывод».
type Block struct {
	Topic      string `json:"topic"`
	Conclusion string `json:"conclusion"`
}

// Week — недельный анализ в API.
type Week struct {
	WeekStart string      `json:"weekStart"` // понедельник
	WeekEnd   string      `json:"weekEnd"`   // воскресенье
	Blocks    []Block     `json:"blocks"`
	UpdatedAt *time.Time  `json:"updatedAt"` // nil — анализ ещё не писали
	Summary   WeekSummary `json:"summary"`
}

// WeekSummary — сводка недели рядом с анализом: дни, средние, привычки, цели.
type WeekSummary struct {
	Days []DaySummary `json:"days"`
	PeriodAverages
	Habits []habits.PeriodStat `json:"habits"`
	Goals  []habits.Goal       `json:"goals"`
}

// PeriodAverages — средние за период по дням не позже upTo (см. Averages).
type PeriodAverages struct {
	Filled      int      `json:"filled"`      // дней с записями
	AvgRating   *float64 `json:"avgRating"`   // по оценённым дням
	AvgDopamine *float64 `json:"avgDopamine"` // по дням с записями: пустой день — «не вёл», а не 0
}

func isFilled(d DaySummary) bool {
	return d.Rating != nil || d.DopamineLevel > 0 || d.Wins > 0 || d.HasStory
}

// Averages — те же правила, что у фронта (lib/journal.summarize): будущие дни не считаются.
func Averages(ds []DaySummary, upTo string) PeriodAverages {
	var a PeriodAverages
	var ratingSum, dopSum, rated int
	for _, d := range ds {
		if d.Day > upTo || !isFilled(d) {
			continue
		}
		a.Filled++
		dopSum += d.DopamineLevel
		if d.Rating != nil {
			rated++
			ratingSum += *d.Rating
		}
	}
	if rated > 0 {
		v := float64(ratingSum) / float64(rated)
		a.AvgRating = &v
	}
	if a.Filled > 0 {
		v := float64(dopSum) / float64(a.Filled)
		a.AvgDopamine = &v
	}
	return a
}

// NormalizeBlocks: «О чём» — trim и схлопнутые пробелы, «Вывод» — trim по краям
// (переносы внутри сохраняются); совсем пустые блоки выкидываются.
func NormalizeBlocks(bs []Block) ([]Block, error) {
	out := make([]Block, 0, len(bs))
	for _, b := range bs {
		b.Topic = strings.Join(strings.Fields(b.Topic), " ")
		b.Conclusion = strings.TrimSpace(b.Conclusion)
		if b.Topic == "" && b.Conclusion == "" {
			continue
		}
		if utf8.RuneCountInString(b.Topic) > MaxTopicLen || utf8.RuneCountInString(b.Conclusion) > MaxConclusionLen {
			return nil, httpx.BadRequest("text_too_long", fmt.Sprintf("topic ≤ %d, conclusion ≤ %d characters", MaxTopicLen, MaxConclusionLen))
		}
		out = append(out, b)
	}
	if len(out) > MaxBlocks {
		return nil, httpx.BadRequest("too_many_items", fmt.Sprintf("at most %d blocks per week", MaxBlocks))
	}
	return out, nil
}

// parseWeek: понедельник из URL; будущие недели — нельзя (как будущие дни).
func (s *Service) parseWeek(mondayS string, write bool) (time.Time, error) {
	monday, err := days.ParseWeek(mondayS)
	if err != nil {
		return time.Time{}, err
	}
	if write {
		if err := days.CheckWritable(monday, days.WeekStart(clock.TodayDate(s.clock))); err != nil {
			return time.Time{}, err
		}
	}
	return monday, nil
}

// Week — анализ недели со сводкой. Недели без анализа нет — отдаём пустую (как день).
func (s *Service) Week(ctx context.Context, mondayS string) (Week, error) {
	monday, err := s.parseWeek(mondayS, false)
	if err != nil {
		return Week{}, err
	}
	w, err := s.loadWeek(ctx, store.New(s.pool), monday)
	if err != nil {
		return Week{}, fmt.Errorf("journal week %s: %w", mondayS, err)
	}
	if w.Summary, err = s.summary(ctx, monday); err != nil {
		return Week{}, fmt.Errorf("journal week %s: %w", mondayS, err)
	}
	return w, nil
}

// SaveWeek заменяет блоки целиком; пустой список удаляет анализ недели.
func (s *Service) SaveWeek(ctx context.Context, mondayS string, blocks []Block) (Week, error) {
	monday, err := s.parseWeek(mondayS, true)
	if err != nil {
		return Week{}, err
	}
	if blocks, err = NormalizeBlocks(blocks); err != nil {
		return Week{}, err
	}
	var w Week
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if len(blocks) == 0 {
			if err := q.DeleteWeek(ctx, monday); err != nil {
				return fmt.Errorf("delete: %w", err)
			}
		} else {
			if err := q.UpsertWeek(ctx, store.UpsertWeekParams{WeekStart: monday, Now: s.clock.Now()}); err != nil {
				return fmt.Errorf("upsert: %w", err)
			}
			if err := q.DeleteWeekBlocks(ctx, monday); err != nil {
				return fmt.Errorf("delete blocks: %w", err)
			}
			for i, b := range blocks {
				if err := q.InsertWeekBlock(ctx, store.InsertWeekBlockParams{
					WeekStart: monday, Position: int16(i), Topic: b.Topic, Conclusion: b.Conclusion,
				}); err != nil {
					return fmt.Errorf("insert block: %w", err)
				}
			}
		}
		w, err = s.loadWeek(ctx, q, monday)
		return err
	})
	if err != nil {
		return Week{}, fmt.Errorf("journal save week %s: %w", mondayS, err)
	}
	if w.Summary, err = s.summary(ctx, monday); err != nil {
		return Week{}, fmt.Errorf("journal save week %s: %w", mondayS, err)
	}
	return w, nil
}

func (s *Service) loadWeek(ctx context.Context, q *store.Queries, monday time.Time) (Week, error) {
	w := Week{WeekStart: clock.FormatDate(monday), WeekEnd: clock.FormatDate(monday.AddDate(0, 0, 6)), Blocks: []Block{}}
	row, err := q.GetWeek(ctx, monday)
	if errors.Is(err, pgx.ErrNoRows) {
		return w, nil
	}
	if err != nil {
		return Week{}, fmt.Errorf("get week: %w", err)
	}
	w.UpdatedAt = &row.UpdatedAt
	bs, err := q.ListWeekBlocks(ctx, monday)
	if err != nil {
		return Week{}, fmt.Errorf("list blocks: %w", err)
	}
	for _, b := range bs {
		w.Blocks = append(w.Blocks, Block{Topic: b.Topic, Conclusion: b.Conclusion})
	}
	return w, nil
}

// summary — дни недели, средние, привычки и цели. Только чтение, вне транзакции записи.
func (s *Service) summary(ctx context.Context, monday time.Time) (WeekSummary, error) {
	sunday := monday.AddDate(0, 0, 6)
	ds, err := s.summaries(ctx, store.New(s.pool), monday, sunday)
	if err != nil {
		return WeekSummary{}, err
	}
	sum := WeekSummary{
		Days:           ds,
		PeriodAverages: Averages(ds, clock.FormatDate(clock.TodayDate(s.clock))),
		Habits:         []habits.PeriodStat{},
		Goals:          []habits.Goal{},
	}
	if s.habits == nil {
		return sum, nil // в unit-тестах хендлеров привычек нет
	}
	if sum.Habits, err = s.habits.PeriodStats(ctx, monday, sunday); err != nil {
		return WeekSummary{}, fmt.Errorf("habit stats: %w", err)
	}
	if sum.Goals, err = s.habits.Goals(ctx, clock.FormatDate(monday), clock.FormatDate(sunday)); err != nil {
		return WeekSummary{}, fmt.Errorf("goals: %w", err)
	}
	return sum, nil
}
