package habits

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lexis/internal/habits/store"
	"lexis/internal/platform/clock"
	"lexis/internal/platform/days"
	"lexis/internal/platform/db"
	"lexis/internal/platform/httpx"
)

const (
	defaultRange   = 30 // окно отметок по умолчанию — последние 30 дней
	defaultNotes   = 20
	maxNotesPerReq = 100
)

type Service struct {
	pool  *pgxpool.Pool
	clock clock.Clock
}

func NewService(pool *pgxpool.Pool, clk clock.Clock) *Service {
	return &Service{pool: pool, clock: clk}
}

var (
	errHabitNotFound = httpx.NotFound("habit not found")
	errDuplicate     = httpx.Conflict("duplicate_habit", "a habit with this name already exists")
	errInvalidHabit  = httpx.BadRequest("invalid_habit", "unknown habit")
)

func (s *Service) toHabit(h store.Habit) Habit {
	return Habit{
		ID: h.ID, Name: h.Name, Kind: h.Kind, Color: h.Color,
		TrackTrigger: h.TrackTrigger, TrackRating: h.TrackRating, TrackAmount: h.TrackAmount, Unit: h.Unit,
		Archived: h.ArchivedAt != nil,
		// pgx отдаёт timestamptz не в TZ пользователя — приводим перед взятием даты (§14).
		StartDay:  clock.FormatDate(clock.DateOf(h.CreatedAt.In(s.clock.Location()))),
		CreatedAt: h.CreatedAt,
	}
}

func fieldsOf(h store.Habit) Fields {
	return Fields{Name: h.Name, Kind: h.Kind, Color: h.Color, TrackTrigger: h.TrackTrigger,
		TrackRating: h.TrackRating, TrackAmount: h.TrackAmount, Unit: h.Unit}
}

// ---------- привычки ----------

// List — привычки по порядку; архивные — только если withArchived.
func (s *Service) List(ctx context.Context, withArchived bool) ([]Habit, error) {
	rows, err := store.New(s.pool).ListHabits(ctx)
	if err != nil {
		return nil, fmt.Errorf("habits list: %w", err)
	}
	out := make([]Habit, 0, len(rows))
	for _, h := range rows {
		if withArchived || h.ArchivedAt == nil {
			out = append(out, s.toHabit(h))
		}
	}
	return out, nil
}

func (s *Service) Create(ctx context.Context, f Fields) (Habit, error) {
	f, err := f.Normalize()
	if err != nil {
		return Habit{}, err
	}
	var h store.Habit
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		pos, err := q.NextHabitPosition(ctx)
		if err != nil {
			return fmt.Errorf("position: %w", err)
		}
		h, err = q.CreateHabit(ctx, store.CreateHabitParams{
			Name: f.Name, Kind: f.Kind, Color: f.Color, TrackTrigger: f.TrackTrigger, TrackRating: f.TrackRating,
			TrackAmount: f.TrackAmount, Unit: f.Unit, Position: pos, CreatedAt: s.clock.Now(),
		})
		return err
	})
	if db.IsUniqueViolation(err) {
		return Habit{}, errDuplicate
	}
	if err != nil {
		return Habit{}, fmt.Errorf("habit create: %w", err)
	}
	return s.toHabit(h), nil
}

func (s *Service) Update(ctx context.Context, id int64, p Patch) (Habit, error) {
	var h store.Habit
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		cur, err := q.GetHabit(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return errHabitNotFound
		}
		if err != nil {
			return fmt.Errorf("get: %w", err)
		}
		f, archivedAt, err := p.Apply(fieldsOf(cur), cur.ArchivedAt, s.clock.Now())
		if err != nil {
			return err
		}
		h, err = q.UpdateHabit(ctx, store.UpdateHabitParams{
			ID: id, Name: f.Name, Color: f.Color, TrackTrigger: f.TrackTrigger, TrackRating: f.TrackRating,
			TrackAmount: f.TrackAmount, Unit: f.Unit, ArchivedAt: archivedAt,
		})
		return err
	})
	if db.IsUniqueViolation(err) {
		return Habit{}, errDuplicate
	}
	if err != nil {
		return Habit{}, fmt.Errorf("habit update %d: %w", id, err)
	}
	return s.toHabit(h), nil
}

// Delete удаляет привычку с отметками и целями; наблюдения о ней становятся общими.
func (s *Service) Delete(ctx context.Context, id int64) error {
	n, err := store.New(s.pool).DeleteHabit(ctx, id)
	if err != nil {
		return fmt.Errorf("habit delete %d: %w", id, err)
	}
	if n == 0 {
		return errHabitNotFound
	}
	return nil
}

// Reorder — новый порядок всех привычек (архивных тоже: ids — полный список).
func (s *Service) Reorder(ctx context.Context, ids []int64) error {
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		rows, err := q.ListHabits(ctx)
		if err != nil {
			return fmt.Errorf("list: %w", err)
		}
		existing := make([]int64, len(rows))
		for i, h := range rows {
			existing[i] = h.ID
		}
		if err := CheckOrder(ids, existing); err != nil {
			return err
		}
		for i, id := range ids {
			if err := q.SetHabitPosition(ctx, store.SetHabitPositionParams{ID: id, Position: int32(i)}); err != nil {
				return fmt.Errorf("set position: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("habits reorder: %w", err)
	}
	return nil
}

// ---------- отметки ----------

func toCheckin(c store.HabitCheckin) Checkin {
	out := Checkin{HabitID: c.HabitID, Day: clock.FormatDate(c.Day), Trigger: c.Trigger, Amount: c.Amount}
	if c.Rating != nil {
		r := int(*c.Rating)
		out.Rating = &r
	}
	return out
}

// Checkins — все отметки в окне ?from=&to= (по умолчанию последние 30 дней).
func (s *Service) Checkins(ctx context.Context, fromS, toS string) (from, to time.Time, out []Checkin, err error) {
	from, to, err = days.ParseRange(fromS, toS, clock.TodayDate(s.clock), defaultRange)
	if err != nil {
		return
	}
	out, err = s.checkins(ctx, store.New(s.pool), from, to)
	return
}

func (s *Service) checkins(ctx context.Context, q *store.Queries, from, to time.Time) ([]Checkin, error) {
	rows, err := q.ListCheckins(ctx, store.ListCheckinsParams{FromDay: from, ToDay: to})
	if err != nil {
		return nil, fmt.Errorf("list checkins: %w", err)
	}
	out := make([]Checkin, len(rows))
	for i, r := range rows {
		out[i] = toCheckin(r)
	}
	return out, nil
}

// SaveDay — форма «Отметки» за день: каждая привычка из списка отмечается или снимается,
// остальные не трогаются. Одна транзакция: ошибка в любой строке — не сохраняется ничего.
func (s *Service) SaveDay(ctx context.Context, dayS string, in []CheckinInput) ([]Checkin, error) {
	day, err := days.Parse(dayS)
	if err != nil {
		return nil, err
	}
	if err := days.CheckWritable(day, clock.TodayDate(s.clock)); err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(in))
	seen := make(map[int64]bool, len(in))
	for _, c := range in {
		if seen[c.HabitID] {
			return nil, httpx.BadRequest("invalid_habit", fmt.Sprintf("habit %d is listed twice", c.HabitID))
		}
		seen[c.HabitID] = true
		ids = append(ids, c.HabitID)
	}

	var out []Checkin
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		hs, err := q.GetHabitsByIDs(ctx, ids)
		if err != nil {
			return fmt.Errorf("get habits: %w", err)
		}
		tracks := make(map[int64]Tracks, len(hs))
		for _, h := range hs {
			tracks[h.ID] = Tracks{Trigger: h.TrackTrigger, Rating: h.TrackRating, Amount: h.TrackAmount}
		}
		now := s.clock.Now()
		for _, c := range in {
			t, ok := tracks[c.HabitID]
			if !ok {
				return errInvalidHabit
			}
			c, err := ValidateCheckin(t, c)
			if err != nil {
				return err
			}
			if !c.Done {
				if err := q.DeleteCheckin(ctx, store.DeleteCheckinParams{HabitID: c.HabitID, Day: day}); err != nil {
					return fmt.Errorf("delete checkin: %w", err)
				}
				continue
			}
			var rating *int16
			if c.Rating != nil {
				r := int16(*c.Rating) // 1..10 — проверено
				rating = &r
			}
			if err := q.UpsertCheckin(ctx, store.UpsertCheckinParams{
				HabitID: c.HabitID, Day: day, Trigger: c.Trigger, Rating: rating, Amount: c.Amount, Now: now,
			}); err != nil {
				return fmt.Errorf("upsert checkin: %w", err)
			}
		}
		out, err = s.checkins(ctx, q, day, day)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("habits save day %s: %w", dayS, err)
	}
	return out, nil
}

// ---------- цели ----------

// Goals — цели, пересекающие окно ?from=&to=; без from и to — все (новые сверху).
func (s *Service) Goals(ctx context.Context, fromS, toS string) ([]Goal, error) {
	q := store.New(s.pool)
	var rows []store.HabitGoal
	var err error
	if fromS == "" && toS == "" {
		rows, err = q.ListAllGoals(ctx)
	} else {
		var from, to time.Time
		if from, to, err = days.ParseRange(fromS, toS, clock.TodayDate(s.clock), defaultRange); err != nil {
			return nil, err
		}
		rows, err = q.ListGoals(ctx, store.ListGoalsParams{FromDay: from, ToDay: to})
	}
	if err != nil {
		return nil, fmt.Errorf("goals list: %w", err)
	}
	return s.withProgress(ctx, q, rows)
}

// withProgress считает прогресс целей с привычкой одним запросом отметок на все цели.
func (s *Service) withProgress(ctx context.Context, q *store.Queries, rows []store.HabitGoal) ([]Goal, error) {
	out := make([]Goal, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	today := clock.TodayDate(s.clock)
	var ids []int64
	from, to := rows[0].StartDay, rows[0].EndDay
	for _, g := range rows {
		if g.HabitID != nil {
			ids = append(ids, *g.HabitID)
		}
		if g.StartDay.Before(from) {
			from = g.StartDay
		}
		if g.EndDay.After(to) {
			to = g.EndDay
		}
	}

	kinds := map[int64]string{}
	checked := map[int64]map[string]bool{}
	if len(ids) > 0 {
		hs, err := q.GetHabitsByIDs(ctx, ids)
		if err != nil {
			return nil, fmt.Errorf("goal habits: %w", err)
		}
		for _, h := range hs {
			kinds[h.ID] = h.Kind
		}
		cs, err := q.ListCheckinDays(ctx, store.ListCheckinDaysParams{HabitIds: ids, FromDay: from, ToDay: to})
		if err != nil {
			return nil, fmt.Errorf("goal checkins: %w", err)
		}
		for _, c := range cs {
			if checked[c.HabitID] == nil {
				checked[c.HabitID] = map[string]bool{}
			}
			checked[c.HabitID][clock.FormatDate(c.Day)] = true
		}
	}

	for i, g := range rows {
		out[i] = Goal{ID: g.ID, HabitID: g.HabitID, Title: g.Title, From: clock.FormatDate(g.StartDay), To: clock.FormatDate(g.EndDay)}
		if g.HabitID != nil {
			p := GoalProgress(kinds[*g.HabitID], g.StartDay, g.EndDay, today, checked[*g.HabitID])
			out[i].Progress = &p
		}
	}
	return out, nil
}

// habitExists — для целей и заметок: неизвестная привычка — 400, а не 500 от внешнего ключа.
func habitExists(ctx context.Context, q *store.Queries, id *int64) error {
	if id == nil {
		return nil
	}
	_, err := q.GetHabit(ctx, *id)
	if errors.Is(err, pgx.ErrNoRows) {
		return errInvalidHabit
	}
	return err
}

func (s *Service) CreateGoal(ctx context.Context, in GoalInput) (Goal, error) {
	g, err := in.Normalize()
	if err != nil {
		return Goal{}, err
	}
	return s.saveGoal(ctx, func(q *store.Queries) (store.HabitGoal, error) {
		return q.CreateGoal(ctx, store.CreateGoalParams{HabitID: g.HabitID, Title: g.Title, StartDay: g.Start, EndDay: g.End, CreatedAt: s.clock.Now()})
	}, g.HabitID)
}

// UpdateGoal заменяет цель целиком (PUT).
func (s *Service) UpdateGoal(ctx context.Context, id int64, in GoalInput) (Goal, error) {
	g, err := in.Normalize()
	if err != nil {
		return Goal{}, err
	}
	return s.saveGoal(ctx, func(q *store.Queries) (store.HabitGoal, error) {
		row, err := q.UpdateGoal(ctx, store.UpdateGoalParams{ID: id, HabitID: g.HabitID, Title: g.Title, StartDay: g.Start, EndDay: g.End})
		if errors.Is(err, pgx.ErrNoRows) {
			return row, httpx.NotFound("goal not found")
		}
		return row, err
	}, g.HabitID)
}

func (s *Service) saveGoal(ctx context.Context, write func(q *store.Queries) (store.HabitGoal, error), habitID *int64) (Goal, error) {
	var out Goal
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := habitExists(ctx, q, habitID); err != nil {
			return err
		}
		row, err := write(q)
		if err != nil {
			return err
		}
		goals, err := s.withProgress(ctx, q, []store.HabitGoal{row})
		if err != nil {
			return err
		}
		out = goals[0]
		return nil
	})
	if err != nil {
		return Goal{}, fmt.Errorf("goal save: %w", err)
	}
	return out, nil
}

func (s *Service) DeleteGoal(ctx context.Context, id int64) error {
	n, err := store.New(s.pool).DeleteGoal(ctx, id)
	if err != nil {
		return fmt.Errorf("goal delete %d: %w", id, err)
	}
	if n == 0 {
		return httpx.NotFound("goal not found")
	}
	return nil
}

// ---------- наблюдения ----------

// NoteFilter — параметры ленты. HabitID = 0 — все; General — только общие записи.
type NoteFilter struct {
	HabitID int64
	General bool
	Kind    string
	Before  int64 // курсор: id последней показанной записи; 0 — с начала
	Limit   int
}

func toNote(n store.HabitNote) Note {
	return Note{ID: n.ID, HabitID: n.HabitID, Kind: n.Kind, Body: n.Body, CreatedAt: n.CreatedAt, UpdatedAt: n.UpdatedAt}
}

// Notes — страница ленты (новые сверху) и курсор следующей (nil — дальше пусто).
func (s *Service) Notes(ctx context.Context, f NoteFilter) ([]Note, *int64, error) {
	if f.Kind != "" && f.Kind != NoteObservation && f.Kind != NoteInsight {
		return nil, nil, httpx.BadRequest("invalid_kind", "kind must be observation or insight")
	}
	if f.Limit <= 0 || f.Limit > maxNotesPerReq {
		f.Limit = defaultNotes
	}
	// Берём на одну больше: так узнаём, есть ли следующая страница, без COUNT.
	rows, err := store.New(s.pool).ListNotes(ctx, store.ListNotesParams{
		Before: f.Before, HabitID: f.HabitID, General: f.General, Kind: f.Kind, Lim: int32(f.Limit + 1),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("notes list: %w", err)
	}
	var next *int64
	if len(rows) > f.Limit {
		rows = rows[:f.Limit]
		id := rows[len(rows)-1].ID
		next = &id
	}
	out := make([]Note, len(rows))
	for i, r := range rows {
		out[i] = toNote(r)
	}
	return out, next, nil
}

func (s *Service) CreateNote(ctx context.Context, in NoteInput) (Note, error) {
	in, err := in.Normalize()
	if err != nil {
		return Note{}, err
	}
	return s.saveNote(ctx, in.HabitID, func(q *store.Queries) (store.HabitNote, error) {
		return q.CreateNote(ctx, store.CreateNoteParams{HabitID: in.HabitID, Kind: in.Kind, Body: in.Body, Now: s.clock.Now()})
	})
}

// UpdateNote заменяет запись целиком (PUT); дата создания не меняется.
func (s *Service) UpdateNote(ctx context.Context, id int64, in NoteInput) (Note, error) {
	in, err := in.Normalize()
	if err != nil {
		return Note{}, err
	}
	return s.saveNote(ctx, in.HabitID, func(q *store.Queries) (store.HabitNote, error) {
		row, err := q.UpdateNote(ctx, store.UpdateNoteParams{ID: id, HabitID: in.HabitID, Kind: in.Kind, Body: in.Body, Now: s.clock.Now()})
		if errors.Is(err, pgx.ErrNoRows) {
			return row, httpx.NotFound("note not found")
		}
		return row, err
	})
}

func (s *Service) saveNote(ctx context.Context, habitID *int64, write func(q *store.Queries) (store.HabitNote, error)) (Note, error) {
	var row store.HabitNote
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := habitExists(ctx, q, habitID); err != nil {
			return err
		}
		var err error
		row, err = write(q)
		return err
	})
	if err != nil {
		return Note{}, fmt.Errorf("note save: %w", err)
	}
	return toNote(row), nil
}

func (s *Service) DeleteNote(ctx context.Context, id int64) error {
	n, err := store.New(s.pool).DeleteNote(ctx, id)
	if err != nil {
		return fmt.Errorf("note delete %d: %w", id, err)
	}
	if n == 0 {
		return httpx.NotFound("note not found")
	}
	return nil
}
