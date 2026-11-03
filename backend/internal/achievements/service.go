package achievements

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"lexis/internal/achievements/store"
	"lexis/internal/platform/clock"
	"lexis/internal/platform/eventbus"
	"lexis/internal/stats"
)

// Service проверяет достижения по текущему состоянию БД и записывает разблокировки.
//
// Проверка идемпотентна и не зависит от того, какое событие её вызвало: «снимок → правила →
// INSERT ON CONFLICT DO NOTHING». Поэтому её можно звать откуда угодно:
//   - из подписчика eventbus (§8: разблокировка через события);
//   - из GET /api/achievements — подписчик асинхронный, и фронт, запросив список сразу
//     после finish, мог бы не увидеть свежую ачивку. Проверка при чтении снимает эту гонку
//     и заодно «долечивает» состояние после импорта/ручных правок БД.
type Service struct {
	pool  *pgxpool.Pool
	clock clock.Clock
}

func NewService(pool *pgxpool.Pool, clk clock.Clock) *Service {
	return &Service{pool: pool, clock: clk}
}

// Achievement — элемент списка для UI. Текст — на фронте по Code (i18n).
type Achievement struct {
	Code       Code       `json:"code"`
	Progress   int        `json:"progress"`
	Target     int        `json:"target"`
	UnlockedAt *time.Time `json:"unlockedAt"` // nil — ещё не получено
}

// Check разблокирует всё, что достигнуто, и возвращает коды, разблокированные именно сейчас.
func (s *Service) Check(ctx context.Context) ([]Code, error) {
	_, unlocked, err := s.check(ctx, store.New(s.pool))
	return unlocked, err
}

func (s *Service) check(ctx context.Context, q *store.Queries) (Snapshot, []Code, error) {
	snap, err := s.snapshot(ctx, q)
	if err != nil {
		return Snapshot{}, nil, fmt.Errorf("achievements check: %w", err)
	}
	now := s.clock.Now()
	var unlocked []Code
	for _, p := range Evaluate(snap) {
		if !p.Reached {
			continue
		}
		n, err := q.UnlockAchievement(ctx, store.UnlockAchievementParams{Code: string(p.Code), Now: now})
		if err != nil {
			return Snapshot{}, nil, fmt.Errorf("achievements check: unlock %s: %w", p.Code, err)
		}
		if n == 1 {
			unlocked = append(unlocked, p.Code)
			slog.InfoContext(ctx, "achievement unlocked", "code", p.Code)
		}
	}
	return snap, unlocked, nil
}

func (s *Service) snapshot(ctx context.Context, q *store.Queries) (Snapshot, error) {
	sessions, err := q.CountFinishedSessions(ctx)
	if err != nil {
		return Snapshot{}, fmt.Errorf("count sessions: %w", err)
	}
	finished, err := q.ListAllFinishedAt(ctx)
	if err != nil {
		return Snapshot{}, fmt.Errorf("finished at: %w", err)
	}
	learned, err := q.CountLearned(ctx)
	if err != nil {
		return Snapshot{}, fmt.Errorf("count learned: %w", err)
	}
	archived, err := q.CountArchivedGroups(ctx)
	if err != nil {
		return Snapshot{}, fmt.Errorf("count archived groups: %w", err)
	}
	return Snapshot{
		Sessions:       int(sessions),
		BestStreak:     stats.Streaks(finished, s.clock.Now()).Best,
		Learned:        int(learned),
		ArchivedGroups: int(archived),
	}, nil
}

// List — все достижения в порядке показа, с прогрессом и датой получения.
// Полученное остаётся полученным, даже если метрика потом упала (слово вернули из архива):
// тогда прогресс показываем полным.
func (s *Service) List(ctx context.Context) ([]Achievement, error) {
	q := store.New(s.pool)
	snap, _, err := s.check(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("achievements list: %w", err)
	}
	rows, err := q.ListUnlocked(ctx)
	if err != nil {
		return nil, fmt.Errorf("achievements list: %w", err)
	}
	at := make(map[Code]time.Time, len(rows))
	for _, r := range rows {
		at[Code(r.Code)] = r.UnlockedAt
	}

	progress := Evaluate(snap)
	out := make([]Achievement, len(progress))
	for i, p := range progress {
		out[i] = Achievement{Code: p.Code, Progress: p.Progress, Target: p.Target}
		if t, ok := at[p.Code]; ok {
			out[i].UnlockedAt = &t
			out[i].Progress = p.Target
		}
	}
	return out, nil
}

// onEvent — подписчик eventbus. Ошибку некуда вернуть, поэтому только лог:
// ничего не теряется, следующая проверка (событие или GET) догонит.
func (s *Service) onEvent(ctx context.Context, e eventbus.Event) {
	switch e.(type) {
	case eventbus.SessionFinished, eventbus.WordLevelChanged:
		if _, err := s.Check(ctx); err != nil {
			slog.ErrorContext(ctx, "achievements: check on event", "event", fmt.Sprintf("%T", e), "err", err)
		}
	}
}
