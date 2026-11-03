-- Запросы модуля importexport: полный бэкап, восстановление, очистка базы.

-- name: ListAllGroups :many
SELECT * FROM groups ORDER BY id;

-- name: ListAllWords :many
SELECT * FROM words ORDER BY id;

-- name: ListAllExamples :many
SELECT word_id, sentence FROM word_examples ORDER BY word_id, position;

-- name: ListAllWordGroups :many
SELECT word_id, group_id FROM word_groups ORDER BY word_id, group_id;

-- name: ListAllSessions :many
SELECT * FROM training_sessions ORDER BY id;

-- name: ListAllReviews :many
SELECT * FROM reviews ORDER BY id;

-- name: ListAllAchievements :many
SELECT * FROM achievements ORDER BY code;

-- name: ListAllSettings :many
SELECT * FROM settings ORDER BY key;

-- name: TruncateData :exec
-- Всё, кроме settings (§12: очистка базы сохраняет язык и лимит). RESTART IDENTITY — id снова с 1.
TRUNCATE words, word_examples, groups, word_groups, training_sessions, reviews, achievements RESTART IDENTITY;

-- name: DeleteSettings :exec
DELETE FROM settings;

-- name: RestoreGroup :one
INSERT INTO groups (name, color, created_at) VALUES (@name, @color, @created_at)
RETURNING id;

-- name: RestoreWord :one
INSERT INTO words (term, forms, translation, level, streak, next_review_at, last_reviewed_at, last_good_at, created_at)
VALUES (@term, @forms, @translation, @level, @streak, @next_review_at, @last_reviewed_at, @last_good_at, @created_at)
RETURNING id;

-- name: RestoreSession :one
INSERT INTO training_sessions (group_id, strategy, free, started_at, finished_at, total, good, hard, again, duration_sec)
VALUES (@group_id, @strategy, @free, @started_at, @finished_at, @total, @good, @hard, @again, @duration_sec)
RETURNING id;

-- name: RestoreExamples :copyfrom
INSERT INTO word_examples (word_id, position, sentence) VALUES (@word_id, @position, @sentence);

-- name: RestoreWordGroups :copyfrom
INSERT INTO word_groups (word_id, group_id) VALUES (@word_id, @group_id);

-- name: RestoreReviews :copyfrom
INSERT INTO reviews (session_id, word_id, answer, first_in_session, level_before, level_after, answered_at)
VALUES (@session_id, @word_id, @answer, @first_in_session, @level_before, @level_after, @answered_at);

-- name: RestoreAchievement :exec
INSERT INTO achievements (code, unlocked_at) VALUES (@code, @unlocked_at);

-- name: RestoreSetting :exec
INSERT INTO settings (key, value) VALUES (@key, @value);
