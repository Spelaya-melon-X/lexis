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

-- Раздел «День» (§17.10): в бэкапе v2 — секция journal.

-- name: ListAllDays :many
SELECT * FROM days ORDER BY day;

-- name: ListAllWins :many
SELECT day, text FROM day_wins ORDER BY day, position;

-- name: ListAllDopamine :many
SELECT * FROM dopamine_slots ORDER BY day, slot;

-- name: TruncateJournal :exec
-- Только если в файле есть секция journal: старый бэкап (v1) дневник не трогает (§17.10).
TRUNCATE days, day_wins, dopamine_slots, week_reviews, week_review_blocks RESTART IDENTITY;

-- name: RestoreDays :copyfrom
INSERT INTO days (day, rating, story, created_at, updated_at) VALUES (@day, @rating, @story, @created_at, @updated_at);

-- name: RestoreWins :copyfrom
INSERT INTO day_wins (day, position, text) VALUES (@day, @position, @text);

-- name: RestoreDopamine :copyfrom
INSERT INTO dopamine_slots (day, slot, text, updated_at) VALUES (@day, @slot, @text, @updated_at);

-- Раздел «Привычки» (§17.10): в бэкапе v2 — секция habits.

-- name: ListAllHabits :many
SELECT * FROM habits ORDER BY id;

-- name: ListAllCheckins :many
SELECT * FROM habit_checkins ORDER BY day, habit_id;

-- name: ListAllHabitGoals :many
SELECT * FROM habit_goals ORDER BY id;

-- name: ListAllHabitNotes :many
SELECT * FROM habit_notes ORDER BY id;

-- name: TruncateHabits :exec
-- Только если в файле есть секция habits (§17.10).
TRUNCATE habits, habit_checkins, habit_goals, habit_notes RESTART IDENTITY;

-- name: RestoreHabit :one
INSERT INTO habits (name, kind, color, track_trigger, track_rating, track_amount, unit, position, archived_at, created_at)
VALUES (@name, @kind, @color, @track_trigger, @track_rating, @track_amount, @unit, @position, @archived_at, @created_at)
RETURNING id;

-- name: RestoreCheckins :copyfrom
INSERT INTO habit_checkins (habit_id, day, trigger, rating, amount, created_at, updated_at)
VALUES (@habit_id, @day, @trigger, @rating, @amount, @created_at, @updated_at);

-- name: RestoreHabitGoals :copyfrom
INSERT INTO habit_goals (habit_id, title, start_day, end_day, created_at)
VALUES (@habit_id, @title, @start_day, @end_day, @created_at);

-- name: RestoreHabitNotes :copyfrom
-- COPY вставляет в порядке файла, поэтому новые id сохраняют порядок ленты.
INSERT INTO habit_notes (habit_id, kind, body, created_at, updated_at)
VALUES (@habit_id, @kind, @body, @created_at, @updated_at);

-- Недельный анализ: в бэкапе v2 — journal.weeks.

-- name: ListAllWeeks :many
SELECT * FROM week_reviews ORDER BY week_start;

-- name: ListAllWeekBlocks :many
SELECT week_start, topic, conclusion FROM week_review_blocks ORDER BY week_start, position;

-- name: RestoreWeeks :copyfrom
INSERT INTO week_reviews (week_start, created_at, updated_at) VALUES (@week_start, @created_at, @updated_at);

-- name: RestoreWeekBlocks :copyfrom
INSERT INTO week_review_blocks (week_start, position, topic, conclusion) VALUES (@week_start, @position, @topic, @conclusion);
