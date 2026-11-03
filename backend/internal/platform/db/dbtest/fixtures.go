package dbtest

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Fixtures — прямые INSERT-ы для подготовки данных в интеграционных тестах:
// так можно точно задать уровни, даты и итоги сессий, не проходя через сервисы.
type Fixtures struct {
	Pool *pgxpool.Pool
	Now  time.Time // created_at для групп/слов
}

func (f Fixtures) Exec(t testing.TB, sql string, args ...any) {
	t.Helper()
	if _, err := f.Pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("dbtest: exec %q: %v", sql, err)
	}
}

// Int — первая колонка первой строки (count(*), RETURNING id).
func (f Fixtures) Int(t testing.TB, sql string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := f.Pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("dbtest: query %q: %v", sql, err)
	}
	return n
}

func (f Fixtures) Group(t testing.TB, name string) int64 {
	t.Helper()
	return f.Int(t, `INSERT INTO groups (name, created_at) VALUES ($1, $2) RETURNING id`, name, f.Now)
}

// Word — слово с переводом "tr_<term>" на уровне level, в перечисленных группах.
func (f Fixtures) Word(t testing.TB, term string, level int16, nextReview time.Time, groupIDs ...int64) int64 {
	t.Helper()
	id := f.Int(t, `INSERT INTO words (term, translation, level, next_review_at, created_at)
	                VALUES ($1, $2, $3, $4, $5) RETURNING id`, term, "tr_"+term, level, nextReview, f.Now)
	for _, g := range groupIDs {
		f.Exec(t, `INSERT INTO word_groups (word_id, group_id) VALUES ($1, $2)`, id, g)
	}
	return id
}

// Session — итоги сессии как после training.Finish.
type Session struct {
	GroupID     *int64
	Free        bool
	FinishedAt  *time.Time // nil — не завершена
	Total       int
	Good        int
	Hard        int
	Again       int
	DurationSec int
}

func (f Fixtures) Session(t testing.TB, s Session) int64 {
	t.Helper()
	started := f.Now.AddDate(0, 0, -400)
	if s.FinishedAt != nil {
		started = s.FinishedAt.Add(-time.Duration(s.DurationSec) * time.Second)
	}
	return f.Int(t, `INSERT INTO training_sessions
	                 (group_id, strategy, free, started_at, finished_at, total, good, hard, again, duration_sec)
	                 VALUES ($1, 'en_ru', $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`,
		s.GroupID, s.Free, started, s.FinishedAt, s.Total, s.Good, s.Hard, s.Again, s.DurationSec)
}

// Review — первый (first=true) или повторный ответ на слово в сессии.
func (f Fixtures) Review(t testing.TB, sessionID, wordID int64, answer string, first bool, at time.Time) {
	t.Helper()
	f.Exec(t, `INSERT INTO reviews (session_id, word_id, answer, first_in_session, level_before, level_after, answered_at)
	           VALUES ($1, $2, $3, $4, 2, 2, $5)`, sessionID, wordID, answer, first, at)
}
