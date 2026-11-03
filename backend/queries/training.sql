-- Запросы модуля training. Время всегда приходит параметром из platform/clock.

-- name: GroupExists :one
SELECT EXISTS (SELECT 1 FROM groups WHERE id = $1);

-- name: ListSessionWords :many
-- Кандидаты в сессию. Обычная: не архив и (новое или due до конца сегодняшнего дня);
-- free: все слова группы. Раскладку по корзинам делает training.Build.
SELECT w.id, w.term, w.forms, w.translation, w.level, w.next_review_at
FROM words w
JOIN word_groups wg ON wg.word_id = w.id
WHERE wg.group_id = @group_id
  AND (@free::bool OR (w.level < 5 AND (w.level = 1 OR w.next_review_at < @day_end)));

-- name: ListExamplesByWordIDs :many
SELECT word_id, sentence FROM word_examples
WHERE word_id = ANY(@word_ids::bigint[])
ORDER BY word_id, position;

-- name: GetSetting :one
SELECT value FROM settings WHERE key = $1;

-- name: InsertSession :one
INSERT INTO training_sessions (group_id, strategy, free, started_at)
VALUES (@group_id, @strategy, @free, @now)
RETURNING id;

-- name: GetSessionForUpdate :one
-- FOR UPDATE: answer и finish одной сессии не должны идти параллельно (двойной клик).
SELECT * FROM training_sessions WHERE id = $1 FOR UPDATE;

-- name: WordInGroup :one
SELECT EXISTS (SELECT 1 FROM word_groups WHERE word_id = @word_id AND group_id = @group_id);

-- name: GetWordSRSForUpdate :one
SELECT id, level, streak, next_review_at, last_reviewed_at, last_good_at
FROM words WHERE id = $1 FOR UPDATE;

-- name: HasReview :one
SELECT EXISTS (SELECT 1 FROM reviews WHERE session_id = @session_id AND word_id = @word_id);

-- name: UpdateWordSRS :exec
UPDATE words
SET level = @level, streak = @streak, next_review_at = @next_review_at,
    last_reviewed_at = @last_reviewed_at, last_good_at = @last_good_at
WHERE id = @id;

-- name: InsertReview :exec
INSERT INTO reviews (session_id, word_id, answer, first_in_session, level_before, level_after, answered_at)
VALUES (@session_id, @word_id, @answer, @first_in_session, @level_before, @level_after, @now);

-- name: SessionTotals :one
-- Итоги — по первым ответам на слово: именно они двигают уровень и идут в «долю Знаю».
SELECT count(*)::int                                  AS total,
       count(*) FILTER (WHERE answer = 'good')::int   AS good,
       count(*) FILTER (WHERE answer = 'hard')::int   AS hard,
       count(*) FILTER (WHERE answer = 'again')::int  AS again
FROM reviews
WHERE session_id = $1 AND first_in_session;

-- name: ListLevelChanges :many
SELECT r.word_id, w.term, r.level_before, r.level_after
FROM reviews r
JOIN words w ON w.id = r.word_id
WHERE r.session_id = $1 AND r.first_in_session AND r.level_before <> r.level_after
ORDER BY r.answered_at, r.id;

-- name: FinishSession :exec
UPDATE training_sessions
SET finished_at = @now::timestamptz, total = @total, good = @good, hard = @hard, again = @again, duration_sec = @duration_sec
WHERE id = @id;
