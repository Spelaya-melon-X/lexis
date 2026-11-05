package journal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lexis/internal/journal/store"
	"lexis/internal/platform/clock"
	"lexis/internal/platform/days"
)

type Service struct {
	pool   *pgxpool.Pool
	clock  clock.Clock
	habits HabitSource // для сводки недели; nil — без привычек
}

func NewService(pool *pgxpool.Pool, clk clock.Clock, hs HabitSource) *Service {
	return &Service{pool: pool, clock: clk, habits: hs}
}

// Get — запись дня; дня без записи — пустой Day.
func (s *Service) Get(ctx context.Context, dayS string) (Day, error) {
	day, err := days.Parse(dayS)
	if err != nil {
		return Day{}, err
	}
	d, err := load(ctx, store.New(s.pool), day)
	if err != nil {
		return Day{}, fmt.Errorf("journal get: %w", err)
	}
	return d, nil
}

// Update применяет патч одной транзакцией и возвращает итоговый день.
func (s *Service) Update(ctx context.Context, dayS string, p Patch) (Day, error) {
	p, err := p.Normalize()
	if err != nil {
		return Day{}, err
	}
	return s.write(ctx, dayS, func(q *store.Queries, day time.Time) error {
		if p.Rating.Set {
			var r *int16
			if p.Rating.Value != nil {
				v := int16(*p.Rating.Value) // 1..10 проверено в Normalize
				r = &v
			}
			if err := q.SetRating(ctx, store.SetRatingParams{Day: day, Rating: r}); err != nil {
				return fmt.Errorf("rating: %w", err)
			}
		}
		if p.Story != nil {
			if err := q.SetStory(ctx, store.SetStoryParams{Day: day, Story: *p.Story}); err != nil {
				return fmt.Errorf("story: %w", err)
			}
		}
		if p.Wins != nil {
			// Список целиком: удалить и вставить заново проще, чем сводить разницу,
			// а пунктов за день — единицы.
			if err := q.DeleteWins(ctx, day); err != nil {
				return fmt.Errorf("delete wins: %w", err)
			}
			for i, w := range *p.Wins {
				if err := q.InsertWin(ctx, store.InsertWinParams{Day: day, Position: int16(i), Text: w}); err != nil {
					return fmt.Errorf("insert win: %w", err)
				}
			}
		}
		return nil
	})
}

// SetDopamine зажигает (text != "") или гасит (text == "") квадратик slot.
func (s *Service) SetDopamine(ctx context.Context, dayS string, slot int, text string) (Day, error) {
	text, err := NormalizeDopamine(slot, text)
	if err != nil {
		return Day{}, err
	}
	return s.write(ctx, dayS, func(q *store.Queries, day time.Time) error {
		if text == "" {
			return q.DeleteDopamine(ctx, store.DeleteDopamineParams{Day: day, Slot: int16(slot)})
		}
		return q.UpsertDopamine(ctx, store.UpsertDopamineParams{Day: day, Slot: int16(slot), Text: text, Now: s.clock.Now()})
	})
}

// write — общий каркас записи: проверка даты, строка дня (EnsureDay — FK для пунктов и
// квадратиков), изменения и чтение итога в одной транзакции.
func (s *Service) write(ctx context.Context, dayS string, fn func(q *store.Queries, day time.Time) error) (Day, error) {
	day, err := days.Parse(dayS)
	if err != nil {
		return Day{}, err
	}
	if err := days.CheckWritable(day, clock.TodayDate(s.clock)); err != nil {
		return Day{}, err
	}
	var out Day
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.EnsureDay(ctx, store.EnsureDayParams{Day: day, Now: s.clock.Now()}); err != nil {
			return fmt.Errorf("ensure day: %w", err)
		}
		if err := fn(q, day); err != nil {
			return err
		}
		out, err = load(ctx, q, day)
		return err
	})
	if err != nil {
		return Day{}, fmt.Errorf("journal write %s: %w", dayS, err)
	}
	return out, nil
}

// List — сводка по каждому дню окна (календарь, графики).
func (s *Service) List(ctx context.Context, fromS, toS string) (from, to time.Time, out []DaySummary, err error) {
	from, to, err = days.ParseRange(fromS, toS, clock.TodayDate(s.clock), defaultRange)
	if err != nil {
		return
	}
	if out, err = s.summaries(ctx, store.New(s.pool), from, to); err != nil {
		err = fmt.Errorf("journal list: %w", err)
	}
	return
}

// summaries — сводка по каждому дню окна [from, to], пустые дни тоже.
func (s *Service) summaries(ctx context.Context, q *store.Queries, from, to time.Time) ([]DaySummary, error) {
	rows, err := q.ListDaySummaries(ctx, store.ListDaySummariesParams{FromDay: from, ToDay: to})
	if err != nil {
		return nil, fmt.Errorf("day summaries: %w", err)
	}
	in := make([]SummaryRow, len(rows))
	for i, r := range rows {
		in[i] = SummaryRow{Day: r.Day, Rating: intPtr(r.Rating), Dopamine: int(r.Dopamine), Wins: int(r.Wins), HasStory: r.HasStory}
	}
	return FillDays(from, to, in), nil
}

func load(ctx context.Context, q *store.Queries, day time.Time) (Day, error) {
	out := emptyDay(day)
	row, err := q.GetDay(ctx, day)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return Day{}, fmt.Errorf("get day: %w", err)
	}
	out.Rating = intPtr(row.Rating)
	out.Story = row.Story

	wins, err := q.ListWins(ctx, day)
	if err != nil {
		return Day{}, fmt.Errorf("list wins: %w", err)
	}
	if wins != nil {
		out.Wins = wins
	}
	slots, err := q.ListDopamine(ctx, day)
	if err != nil {
		return Day{}, fmt.Errorf("list dopamine: %w", err)
	}
	for _, sl := range slots {
		out.Dopamine[sl.Slot] = &sl.Text // slot 0..6 гарантирует CHECK; sl — своя переменная на итерацию (Go 1.22+)
	}
	out.DopamineLevel = len(slots)
	return out, nil
}

func intPtr(v *int16) *int {
	if v == nil {
		return nil
	}
	n := int(*v)
	return &n
}
