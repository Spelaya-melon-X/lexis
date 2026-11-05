package focus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lexis/internal/focus/store"
	"lexis/internal/platform/clock"
	"lexis/internal/platform/days"
	"lexis/internal/platform/db"
	"lexis/internal/platform/eventbus"
	"lexis/internal/platform/httpx"
	"lexis/internal/tasks"
)

// Tasks — что фокусу нужно от модуля tasks (реализует *tasks.Service): само дело и «выполнено» по «Я закончил».
// focus импортирует tasks только ради типа Task — как journal импортирует habits.
type Tasks interface {
	Get(ctx context.Context, id int64) (tasks.Task, error)
	Complete(ctx context.Context, id int64) error
}

// Dopamine — запись покупки в дофаминовую шкалу дня (B15, реализует *journal.Service).
type Dopamine interface {
	AddDopamine(ctx context.Context, text string) (bool, error)
}

// pointsLockKey — pg_advisory_xact_lock для покупок: «прочитать баланс → списать» атомарно.
const pointsLockKey int64 = 0x6c657869735f7074 // "lexis_pt"

type Service struct {
	pool     *pgxpool.Pool
	clock    clock.Clock
	tasks    Tasks
	dopamine Dopamine
	bus      eventbus.Publisher
}

// NewService: bus — для FocusFinished (достижения «Игры», §18.3 B5).
func NewService(pool *pgxpool.Pool, clk clock.Clock, t Tasks, d Dopamine, bus eventbus.Publisher) *Service {
	return &Service{pool: pool, clock: clk, tasks: t, dopamine: d, bus: bus}
}

var (
	errSessionNotFound = httpx.NotFound("focus session not found")
	errRewardNotFound  = httpx.NotFound("reward not found")
)

// ---------- сессии ----------

type Break struct {
	StartedAt time.Time  `json:"startedAt"`
	EndedAt   *time.Time `json:"endedAt"`
}

type Distraction struct {
	ID      int64     `json:"id"`
	At      time.Time `json:"at"`
	Trigger string    `json:"trigger"`
	GaveIn  bool      `json:"gaveIn"`
}

// Session — фокус-сессия в API. Now — время сервера: фронт считает таймеры от отметок
// с поправкой на разницу часов (§18.3 B4), поэтому перезагрузка ничего не сбивает.
type Session struct {
	ID            int64         `json:"id"`
	TaskID        *int64        `json:"taskId"`
	TaskTitle     string        `json:"taskTitle"`
	Day           string        `json:"day"`
	PlannedBlocks int           `json:"plannedBlocks"`
	StartedAt     time.Time     `json:"startedAt"`
	WorkStartedAt *time.Time    `json:"workStartedAt"`
	FinishedAt    *time.Time    `json:"finishedAt"`
	Outcome       *string       `json:"outcome"`
	Breaks        []Break       `json:"breaks"`
	Distractions  []Distraction `json:"distractions"`
	Timeline      Timeline      `json:"timeline"`
	Points        int           `json:"points"` // начислено за сессию (после завершения)
	Now           time.Time     `json:"now"`
}

// Active — текущая сессия или nil.
func (s *Service) Active(ctx context.Context) (*Session, error) {
	q := store.New(s.pool)
	row, err := q.ActiveSession(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("active session: %w", err)
	}
	sess, err := s.load(ctx, q, row)
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

func (s *Service) Get(ctx context.Context, id int64) (Session, error) {
	q := store.New(s.pool)
	row, err := q.GetSession(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, errSessionNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("get session %d: %w", id, err)
	}
	return s.load(ctx, q, row)
}

// Start начинает сессию по делу: фаза настройки. Только невыполненный лист; одна активная сессия на всё.
// Минимум работы — блоки дела в сегодняшнем плане, а если его там нет — объём дела.
func (s *Service) Start(ctx context.Context, taskID int64) (Session, error) {
	if taskID <= 0 {
		return Session{}, httpx.BadRequest("invalid_task", "taskId must be a positive id")
	}
	t, err := s.tasks.Get(ctx, taskID)
	var he *httpx.Error
	if errors.As(err, &he) && he.Status == 404 {
		return Session{}, httpx.BadRequest("invalid_task", "task not found")
	}
	if err != nil {
		return Session{}, fmt.Errorf("focus start: %w", err)
	}
	switch {
	case !t.Leaf:
		return Session{}, httpx.BadRequest("task_not_leaf", "start a focus session on one of the task steps")
	case t.Done:
		return Session{}, httpx.BadRequest("task_done", "the task is already done")
	}

	var out Session
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		day := clock.TodayDate(s.clock)
		blocks, err := q.PlannedBlocks(ctx, store.PlannedBlocksParams{Day: day, TaskID: taskID})
		if errors.Is(err, pgx.ErrNoRows) {
			blocks, err = int16(min(t.Blocks, tasks.MaxBlocks)), nil
		}
		if err != nil {
			return fmt.Errorf("planned blocks: %w", err)
		}
		row, err := q.CreateSession(ctx, store.CreateSessionParams{
			TaskID: &taskID, TaskTitle: t.Title, Day: day, PlannedBlocks: blocks, Now: s.clock.Now(),
		})
		if db.IsUniqueViolation(err) {
			return httpx.Conflict("focus_active", "another focus session is already running")
		}
		if err != nil {
			return fmt.Errorf("create session: %w", err)
		}
		out, err = s.load(ctx, q, row)
		return err
	})
	if err != nil {
		return Session{}, fmt.Errorf("focus start: %w", err)
	}
	return out, nil
}

// Act — «Начал делать» / «Перерыв» / «Вернулся».
func (s *Service) Act(ctx context.Context, id int64, a Action) (Session, error) {
	return s.change(ctx, id, func(q *store.Queries, row store.FocusSession, tl Timeline, now time.Time) error {
		if err := CheckAction(tl.Phase, a); err != nil {
			return err
		}
		switch a {
		case ActionWork:
			return q.StartWork(ctx, store.StartWorkParams{ID: id, Now: now})
		case ActionBreak:
			return q.StartBreak(ctx, store.StartBreakParams{SessionID: id, Now: now})
		case ActionResume:
			return q.EndBreak(ctx, store.EndBreakParams{SessionID: id, Now: now})
		}
		return httpx.BadRequest("invalid_action", "unknown action")
	})
}

// Distract отмечает отвлечение (во время работы или перерыва).
func (s *Service) Distract(ctx context.Context, id int64, in DistractionInput) (Session, error) {
	in, err := in.Normalize()
	if err != nil {
		return Session{}, err
	}
	return s.change(ctx, id, func(q *store.Queries, _ store.FocusSession, tl Timeline, now time.Time) error {
		if err := CheckAction(tl.Phase, ActionDistraction); err != nil {
			return err
		}
		return q.AddDistraction(ctx, store.AddDistractionParams{SessionID: id, Now: now, Trigger: in.Trigger, GaveIn: in.GaveIn})
	})
}

// Finish завершает сессию: закрывает перерыв, начисляет очки (одной записью в журнале), а для «Я закончил»
// после коммита отмечает дело выполненным. Дело к этому времени могли удалить или разбить —
// тогда сессия всё равно завершена, а ошибка только в логе: потерять отработанное время хуже.
func (s *Service) Finish(ctx context.Context, id int64, o Outcome) (Session, error) {
	var taskID *int64
	out, err := s.change(ctx, id, func(q *store.Queries, row store.FocusSession, tl Timeline, now time.Time) error {
		if err := CheckFinish(tl.Phase, o); err != nil {
			return err
		}
		if tl.Phase == PhaseBreak {
			if err := q.EndBreak(ctx, store.EndBreakParams{SessionID: id, Now: now}); err != nil {
				return fmt.Errorf("end break: %w", err)
			}
		}
		if err := q.FinishSession(ctx, store.FinishSessionParams{ID: id, Now: now, Outcome: string(o)}); err != nil {
			return fmt.Errorf("finish: %w", err)
		}
		breaks, err := s.breaks(ctx, q, id)
		if err != nil {
			return err
		}
		final := Compute(Times{StartedAt: row.StartedAt, WorkStartedAt: row.WorkStartedAt, FinishedAt: &now}, breaks, now)
		if pts := Points(final, o); pts > 0 {
			if err := q.AddPoints(ctx, store.AddPointsParams{At: now, Delta: int32(pts), Kind: "focus", Title: row.TaskTitle, FocusSessionID: &id}); err != nil {
				return fmt.Errorf("add points: %w", err)
			}
		}
		taskID = row.TaskID
		return nil
	})
	if err != nil {
		return Session{}, err
	}
	// После коммита: подписчики не должны увидеть сессию, которой в итоге нет.
	if o != OutcomeAbandoned {
		s.bus.Publish(eventbus.FocusFinished{SessionID: id, WorkSec: out.Timeline.WorkSec, Outcome: string(o), At: *out.FinishedAt})
	}
	if o == OutcomeDone && taskID != nil {
		if err := s.tasks.Complete(ctx, *taskID); err != nil {
			slog.WarnContext(ctx, "focus: could not mark task done", "task", *taskID, "session", id, "err", err)
		}
	}
	return out, nil
}

// change — каркас изменения сессии: блокировка строки, расчёт фазы «сейчас», действие и свежий снимок.
func (s *Service) change(ctx context.Context, id int64, fn func(q *store.Queries, row store.FocusSession, tl Timeline, now time.Time) error) (Session, error) {
	var out Session
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.GetSessionForUpdate(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return errSessionNotFound
		}
		if err != nil {
			return fmt.Errorf("get: %w", err)
		}
		breaks, err := s.breaks(ctx, q, id)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		if err := fn(q, row, Compute(timesOf(row), breaks, now), now); err != nil {
			return err
		}
		if row, err = q.GetSession(ctx, id); err != nil {
			return fmt.Errorf("reload: %w", err)
		}
		out, err = s.load(ctx, q, row)
		return err
	})
	if err != nil {
		return Session{}, fmt.Errorf("focus session %d: %w", id, err)
	}
	return out, nil
}

func timesOf(r store.FocusSession) Times {
	return Times{StartedAt: r.StartedAt, WorkStartedAt: r.WorkStartedAt, FinishedAt: r.FinishedAt}
}

func (s *Service) breaks(ctx context.Context, q *store.Queries, id int64) ([]Span, error) {
	rows, err := q.ListBreaks(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("breaks: %w", err)
	}
	out := make([]Span, len(rows))
	for i, b := range rows {
		out[i] = Span{Start: b.StartedAt, End: b.EndedAt}
	}
	return out, nil
}

// load собирает Session: перерывы, отвлечения, таймлайн на «сейчас» и начисленные очки.
func (s *Service) load(ctx context.Context, q *store.Queries, r store.FocusSession) (Session, error) {
	now := s.clock.Now()
	spans, err := s.breaks(ctx, q, r.ID)
	if err != nil {
		return Session{}, err
	}
	ds, err := q.ListDistractions(ctx, r.ID)
	if err != nil {
		return Session{}, fmt.Errorf("distractions: %w", err)
	}
	pts, err := q.SessionPoints(ctx, &r.ID)
	if err != nil {
		return Session{}, fmt.Errorf("session points: %w", err)
	}
	out := Session{
		ID: r.ID, TaskID: r.TaskID, TaskTitle: r.TaskTitle, Day: clock.FormatDate(r.Day), PlannedBlocks: int(r.PlannedBlocks),
		StartedAt: r.StartedAt, WorkStartedAt: r.WorkStartedAt, FinishedAt: r.FinishedAt, Outcome: r.Outcome,
		Breaks: make([]Break, len(spans)), Distractions: make([]Distraction, len(ds)),
		Timeline: Compute(timesOf(r), spans, now), Points: int(pts), Now: now,
	}
	for i, b := range spans {
		out.Breaks[i] = Break{StartedAt: b.Start, EndedAt: b.End}
	}
	for i, d := range ds {
		out.Distractions[i] = Distraction{ID: d.ID, At: d.At, Trigger: d.Trigger, GaveIn: d.GaveIn}
	}
	return out, nil
}

// Triggers — подсказки для поля «что отвлекло»: частые сверху.
func (s *Service) Triggers(ctx context.Context, query string) ([]string, error) {
	rows, err := store.New(s.pool).Triggers(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("triggers: %w", err)
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Trigger
	}
	return out, nil
}

// ---------- ритуал ----------

func (s *Service) Ritual(ctx context.Context) ([]string, error) {
	rows, err := store.New(s.pool).ListRitual(ctx)
	if err != nil {
		return nil, fmt.Errorf("ritual: %w", err)
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Text
	}
	return out, nil
}

// SaveRitual заменяет чек-лист целиком.
func (s *Service) SaveRitual(ctx context.Context, items []string) ([]string, error) {
	items, err := NormalizeRitual(items)
	if err != nil {
		return nil, err
	}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.DeleteRitual(ctx); err != nil {
			return err
		}
		for i, it := range items {
			if err := q.InsertRitual(ctx, store.InsertRitualParams{Text: it, Position: int32(i)}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("save ritual: %w", err)
	}
	return items, nil
}

// ---------- награды и очки ----------

type Reward struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Price    int    `json:"price"`
	Archived bool   `json:"archived"`
}

func toReward(r store.Reward) Reward {
	return Reward{ID: r.ID, Title: r.Title, Price: int(r.Price), Archived: r.ArchivedAt != nil}
}

// Rewards — по порядку; архивные — только если withArchived.
func (s *Service) Rewards(ctx context.Context, withArchived bool) ([]Reward, error) {
	rows, err := store.New(s.pool).ListRewards(ctx)
	if err != nil {
		return nil, fmt.Errorf("rewards: %w", err)
	}
	out := make([]Reward, 0, len(rows))
	for _, r := range rows {
		if withArchived || r.ArchivedAt == nil {
			out = append(out, toReward(r))
		}
	}
	return out, nil
}

func (s *Service) CreateReward(ctx context.Context, in RewardInput) (Reward, error) {
	in, err := in.Normalize()
	if err != nil {
		return Reward{}, err
	}
	var r store.Reward
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		pos, err := q.NextRewardPosition(ctx)
		if err != nil {
			return err
		}
		r, err = q.CreateReward(ctx, store.CreateRewardParams{Title: in.Title, Price: int32(in.Price), Position: pos, Now: s.clock.Now()})
		return err
	})
	if err != nil {
		return Reward{}, fmt.Errorf("create reward: %w", err)
	}
	return toReward(r), nil
}

func (s *Service) UpdateReward(ctx context.Context, id int64, p RewardPatch) (Reward, error) {
	var r store.Reward
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		cur, err := q.GetReward(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return errRewardNotFound
		}
		if err != nil {
			return err
		}
		in, archivedAt, err := p.Apply(RewardInput{Title: cur.Title, Price: int(cur.Price)}, cur.ArchivedAt, s.clock.Now())
		if err != nil {
			return err
		}
		r, err = q.UpdateReward(ctx, store.UpdateRewardParams{ID: id, Title: in.Title, Price: int32(in.Price), ArchivedAt: archivedAt})
		return err
	})
	if err != nil {
		return Reward{}, fmt.Errorf("update reward %d: %w", id, err)
	}
	return toReward(r), nil
}

func (s *Service) DeleteReward(ctx context.Context, id int64) error {
	n, err := store.New(s.pool).DeleteReward(ctx, id)
	if err != nil {
		return fmt.Errorf("delete reward %d: %w", id, err)
	}
	if n == 0 {
		return errRewardNotFound
	}
	return nil
}

// ReorderRewards — новый порядок всех наград (архивных тоже).
func (s *Service) ReorderRewards(ctx context.Context, ids []int64) error {
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		rows, err := q.ListRewards(ctx)
		if err != nil {
			return err
		}
		existing := make([]int64, len(rows))
		for i, r := range rows {
			existing[i] = r.ID
		}
		if err := tasks.CheckOrder(ids, existing); err != nil {
			return err
		}
		for i, id := range ids {
			if err := q.SetRewardPosition(ctx, store.SetRewardPositionParams{ID: id, Position: int32(i)}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("reorder rewards: %w", err)
	}
	return nil
}

// Purchase — итог покупки. Dopamine: "added" — записано в шкалу дня, "full" — все 7 квадратиков заняты,
// "failed" — не получилось (покупка всё равно прошла), "" — не просили.
type Purchase struct {
	Balance  int    `json:"balance"`
	Dopamine string `json:"dopamine"`
}

// Buy списывает цену награды. Баланс не уходит в минус: чтение и списание — под одной блокировкой.
// Запись в дофамин — после коммита и не откатывает покупку (это приятный бонус, а не часть сделки).
func (s *Service) Buy(ctx context.Context, id int64, toDopamine bool) (Purchase, error) {
	var out Purchase
	var title string
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.LockPoints(ctx, pointsLockKey); err != nil {
			return fmt.Errorf("lock: %w", err)
		}
		r, err := q.GetReward(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return errRewardNotFound
		}
		if err != nil {
			return err
		}
		if r.ArchivedAt != nil {
			return httpx.Conflict("reward_archived", "the reward is archived")
		}
		balance, err := q.Balance(ctx)
		if err != nil {
			return err
		}
		if balance < r.Price {
			return httpx.Conflict("not_enough_points", fmt.Sprintf("the reward costs %d, the balance is %d", r.Price, balance))
		}
		if err := q.AddPoints(ctx, store.AddPointsParams{At: s.clock.Now(), Delta: -r.Price, Kind: "purchase", Title: r.Title, RewardID: &id}); err != nil {
			return fmt.Errorf("spend: %w", err)
		}
		out.Balance, title = int(balance-r.Price), r.Title
		return nil
	})
	if err != nil {
		return Purchase{}, fmt.Errorf("buy reward %d: %w", id, err)
	}
	if toDopamine {
		switch added, err := s.dopamine.AddDopamine(ctx, title); {
		case err != nil:
			slog.WarnContext(ctx, "focus: could not add the purchase to dopamine", "reward", id, "err", err)
			out.Dopamine = "failed"
		case added:
			out.Dopamine = "added"
		default:
			out.Dopamine = "full"
		}
	}
	return out, nil
}

type LedgerEntry struct {
	ID    int64     `json:"id"`
	At    time.Time `json:"at"`
	Delta int       `json:"delta"`
	Kind  string    `json:"kind"`
	Title string    `json:"title"`
}

// Points — баланс и лента журнала (новые сверху, курсор before — id последней показанной записи).
func (s *Service) Points(ctx context.Context, before *int64, limit int) (balance int, entries []LedgerEntry, next *int64, err error) {
	q := store.New(s.pool)
	b, err := q.Balance(ctx)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("balance: %w", err)
	}
	rows, err := q.Ledger(ctx, store.LedgerParams{Before: before, Lim: int32(limit + 1)}) // +1 — есть ли ещё
	if err != nil {
		return 0, nil, nil, fmt.Errorf("ledger: %w", err)
	}
	entries = make([]LedgerEntry, 0, min(len(rows), limit))
	for i, r := range rows {
		if i == limit {
			next = &rows[i-1].ID
			break
		}
		entries = append(entries, LedgerEntry{ID: r.ID, At: r.At, Delta: int(r.Delta), Kind: r.Kind, Title: r.Title})
	}
	return int(b), entries, next, nil
}

// ---------- настройки ----------

func (s *Service) Settings(ctx context.Context) Settings {
	return loadSettings(ctx, store.New(s.pool))
}

func (s *Service) UpdateSettings(ctx context.Context, p SettingsPatch) (Settings, error) {
	next, err := p.Apply(s.Settings(ctx))
	if err != nil {
		return Settings{}, err
	}
	raw, _ := json.Marshal(next.SetupMin)
	if err := store.New(s.pool).PutSetting(ctx, store.PutSettingParams{Key: SettingSetupMin, Value: raw}); err != nil {
		return Settings{}, fmt.Errorf("put setting: %w", err)
	}
	return next, nil
}

// loadSettings: нет ключа или мусор — значение по умолчанию (как у new_words_limit).
func loadSettings(ctx context.Context, q *store.Queries) Settings {
	st := Settings{SetupMin: DefaultSetupMin}
	raw, err := q.GetSetting(ctx, SettingSetupMin)
	if errors.Is(err, pgx.ErrNoRows) {
		return st
	}
	var v int
	if err == nil {
		err = json.Unmarshal(raw, &v)
	}
	if err != nil {
		slog.WarnContext(ctx, "focus: bad setting, using default", "raw", string(raw), "err", err)
		return st
	}
	if checked, err := (SettingsPatch{SetupMin: &v}).Apply(st); err == nil {
		return checked
	}
	return st
}

// ---------- статистика ----------

const defaultStatsDays = 7

// Stats — окно ?from=&to= (по умолчанию последние 7 дней) + рекорды за всё время.
func (s *Service) Stats(ctx context.Context, fromS, toS string) (Stats, error) {
	from, to, err := days.ParseRange(fromS, toS, clock.TodayDate(s.clock), defaultStatsDays)
	if err != nil {
		return Stats{}, err
	}
	q := store.New(s.pool)
	rows, err := q.StatSessions(ctx, store.StatSessionsParams{FromDay: from, ToDay: to})
	if err != nil {
		return Stats{}, fmt.Errorf("stat sessions: %w", err)
	}
	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	breaks, err := q.BreaksOf(ctx, ids)
	if err != nil {
		return Stats{}, fmt.Errorf("breaks: %w", err)
	}
	spans := map[int64][]Span{}
	for _, b := range breaks {
		spans[b.SessionID] = append(spans[b.SessionID], Span{Start: b.StartedAt, End: b.EndedAt})
	}
	ds, err := q.DistractionsOf(ctx, ids)
	if err != nil {
		return Stats{}, fmt.Errorf("distractions: %w", err)
	}
	distr := map[int64][]DistractionStat{}
	for _, d := range ds {
		distr[d.SessionID] = append(distr[d.SessionID], DistractionStat{Trigger: d.Trigger, GaveIn: d.GaveIn})
	}
	now := s.clock.Now()
	sessions := make([]SessionStat, len(rows))
	for i, r := range rows {
		sessions[i] = SessionStat{
			ID: r.ID, TaskTitle: r.TaskTitle, Day: r.Day, Outcome: Outcome(deref(r.Outcome)),
			Timeline: Compute(timesOf(r), spans[r.ID], now), Distractions: distr[r.ID],
		}
	}

	// Окно дат → полуинтервал времени в TZ пользователя: дело, закрытое в 00:30, — «сегодня», а не «вчера».
	loc := s.clock.Location()
	start := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, loc)
	end := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
	doneRows, err := q.DoneTasksBetween(ctx, store.DoneTasksBetweenParams{FromAt: start, ToAt: end})
	if err != nil {
		return Stats{}, fmt.Errorf("done tasks: %w", err)
	}
	done := make([]DoneTask, len(doneRows))
	for i, r := range doneRows {
		done[i] = DoneTask{ID: r.ID, Title: r.Title, Day: clock.DateOf(r.DoneAt.In(loc))}
	}

	st := Summarize(from, to, sessions, done)
	work, err := q.WorkByDay(ctx)
	if err != nil {
		return Stats{}, fmt.Errorf("work by day: %w", err)
	}
	points := make([]DayWork, len(work))
	for i, w := range work {
		points[i] = DayWork{Day: w.Day, WorkSec: int(w.WorkSec)}
	}
	total, err := q.CountDoneTasks(ctx)
	if err != nil {
		return Stats{}, fmt.Errorf("count done: %w", err)
	}
	st.Records = BuildRecords(points, int(total))
	return st, nil
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
