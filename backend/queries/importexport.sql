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

-- name: SetGroupParent :exec
-- Родителей ставим вторым проходом, когда все группы уже вставлены: порядок в файле не важен.
UPDATE groups SET parent_id = @parent_id WHERE id = @id;

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

-- Раздел «Игра в жизнь» (§18.1): в бэкапе v3 — секция game.

-- name: ListAllTaskTypes :many
SELECT * FROM task_types ORDER BY id;

-- name: ListAllTaskColumns :many
SELECT * FROM task_columns ORDER BY id;

-- name: ListAllTasks :many
SELECT * FROM tasks ORDER BY id;

-- name: TruncateGame :exec
-- Только если в файле есть секция game (как habits, §17.10).
TRUNCATE task_types, task_columns, tasks, plan_items, ritual_items, focus_sessions, focus_breaks, focus_distractions, rewards, points_ledger RESTART IDENTITY;

-- name: RestoreTaskType :one
INSERT INTO task_types (name, color, position, created_at) VALUES (@name, @color, @position, @created_at)
RETURNING id;

-- name: RestoreTaskColumn :one
INSERT INTO task_columns (name, position, created_at) VALUES (@name, @position, @created_at)
RETURNING id;

-- name: SetTaskParent :exec
-- Вторым проходом, как у групп: родитель может стоять в файле позже шага.
UPDATE tasks SET parent_id = @parent_id WHERE id = @id;

-- name: RestoreTask :one
-- По одной, а не COPY: для ссылок на родителя нужны новые id.
INSERT INTO tasks (title, blocks, importance, deadline, type_id, column_id, position, note, done_at, created_at, updated_at)
VALUES (@title, @blocks, @importance, @deadline, @type_id, @column_id, @position, @note, @done_at, @created_at, @updated_at)
RETURNING id;

-- name: ListAllPlanItems :many
SELECT * FROM plan_items ORDER BY day, position;

-- name: RestorePlanItems :copyfrom
INSERT INTO plan_items (day, task_id, position, blocks, start_min) VALUES (@day, @task_id, @position, @blocks, @start_min);

-- Фокус и награды (B4) — тоже секция game.

-- name: ListAllRitual :many
SELECT text FROM ritual_items ORDER BY position, id;

-- name: ListAllFocusSessions :many
SELECT * FROM focus_sessions ORDER BY id;

-- name: ListAllFocusBreaks :many
SELECT * FROM focus_breaks ORDER BY session_id, started_at, id;

-- name: ListAllFocusDistractions :many
SELECT * FROM focus_distractions ORDER BY session_id, at, id;

-- name: ListAllRewards :many
SELECT * FROM rewards ORDER BY id;

-- name: ListAllPoints :many
SELECT * FROM points_ledger ORDER BY id;

-- name: RestoreRitual :copyfrom
INSERT INTO ritual_items (text, position) VALUES (@text, @position);

-- name: RestoreFocusSession :one
INSERT INTO focus_sessions (task_id, task_title, day, planned_blocks, started_at, work_started_at, finished_at, outcome)
VALUES (@task_id, @task_title, @day, @planned_blocks, @started_at, @work_started_at, @finished_at, @outcome)
RETURNING id;

-- name: RestoreFocusBreaks :copyfrom
INSERT INTO focus_breaks (session_id, started_at, ended_at) VALUES (@session_id, @started_at, @ended_at);

-- name: RestoreFocusDistractions :copyfrom
INSERT INTO focus_distractions (session_id, at, trigger, gave_in) VALUES (@session_id, @at, @trigger, @gave_in);

-- name: RestoreReward :one
INSERT INTO rewards (title, price, position, archived_at, created_at) VALUES (@title, @price, @position, @archived_at, @created_at)
RETURNING id;

-- name: RestorePoints :copyfrom
-- COPY вставляет в порядке файла — новые id сохраняют порядок ленты.
INSERT INTO points_ledger (at, delta, kind, title, focus_session_id, reward_id)
VALUES (@at, @delta, @kind, @title, @focus_session_id, @reward_id);

-- Раздел «Спорт» (§18.4): в бэкапе v3 — секция sport.

-- name: ListAllExercises :many
SELECT * FROM exercises ORDER BY id;

-- name: ListAllWorkouts :many
SELECT * FROM workouts ORDER BY id;

-- name: ListAllWorkoutExercises :many
SELECT * FROM workout_exercises ORDER BY workout_id, position;

-- name: TruncateSport :exec
-- Только если в файле есть секция sport (как habits, §17.10).
TRUNCATE exercises, workouts, workout_exercises, meals RESTART IDENTITY;

-- name: RestoreExercise :one
INSERT INTO exercises (name, created_at) VALUES (@name, @created_at)
RETURNING id;

-- name: RestoreWorkout :one
INSERT INTO workouts (day, title, notes, duration_min, kcal, created_at, updated_at)
VALUES (@day, @title, @notes, @duration_min, @kcal, @created_at, @updated_at)
RETURNING id;

-- name: RestoreWorkoutExercises :copyfrom
INSERT INTO workout_exercises (workout_id, exercise_id, position, sets, reps, weight)
VALUES (@workout_id, @exercise_id, @position, @sets, @reps, @weight);

-- name: ListAllMeals :many
SELECT * FROM meals ORDER BY day, position;

-- name: RestoreMeals :copyfrom
INSERT INTO meals (day, position, text, kcal, created_at) VALUES (@day, @position, @text, @kcal, @created_at);
