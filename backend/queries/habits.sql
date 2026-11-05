-- Запросы модуля habits (SPEC §17.4).

-- ---------- привычки ----------

-- name: ListHabits :many
SELECT * FROM habits ORDER BY position, id;

-- name: GetHabit :one
SELECT * FROM habits WHERE id = @id;

-- name: GetHabitsByIDs :many
SELECT * FROM habits WHERE id = ANY(@ids::bigint[]);

-- name: NextHabitPosition :one
SELECT COALESCE(max(position), -1)::int + 1 FROM habits;

-- name: CreateHabit :one
INSERT INTO habits (name, kind, color, track_trigger, track_rating, track_amount, unit, position, created_at)
VALUES (@name, @kind, @color, @track_trigger, @track_rating, @track_amount, @unit, @position, @created_at)
RETURNING *;

-- name: UpdateHabit :one
-- Тип (kind) не меняется: смысл старых отметок («сделал» / «сорвался») зависит от него.
UPDATE habits SET name = @name, color = @color, track_trigger = @track_trigger, track_rating = @track_rating,
    track_amount = @track_amount, unit = @unit, archived_at = @archived_at
WHERE id = @id
RETURNING *;

-- name: SetHabitPosition :exec
UPDATE habits SET position = @position WHERE id = @id;

-- name: DeleteHabit :execrows
DELETE FROM habits WHERE id = @id;

-- ---------- отметки ----------

-- name: ListCheckins :many
SELECT * FROM habit_checkins WHERE day BETWEEN @from_day AND @to_day ORDER BY day, habit_id;

-- name: UpsertCheckin :exec
INSERT INTO habit_checkins (habit_id, day, trigger, rating, amount, created_at, updated_at)
VALUES (@habit_id, @day, @trigger, @rating, @amount, @now, @now)
ON CONFLICT (habit_id, day) DO UPDATE
SET trigger = EXCLUDED.trigger, rating = EXCLUDED.rating, amount = EXCLUDED.amount, updated_at = EXCLUDED.updated_at;

-- name: DeleteCheckin :exec
DELETE FROM habit_checkins WHERE habit_id = @habit_id AND day = @day;

-- name: ListCheckinDays :many
-- Дни с отметками по привычкам в окне — для прогресса целей.
SELECT habit_id, day FROM habit_checkins
WHERE habit_id = ANY(@habit_ids::bigint[]) AND day BETWEEN @from_day AND @to_day;

-- ---------- цели ----------

-- name: ListGoals :many
-- Цели, пересекающие окно [from, to].
SELECT * FROM habit_goals WHERE end_day >= @from_day AND start_day <= @to_day ORDER BY start_day, id;

-- name: ListAllGoals :many
SELECT * FROM habit_goals ORDER BY start_day DESC, id DESC;

-- name: GetGoal :one
SELECT * FROM habit_goals WHERE id = @id;

-- name: CreateGoal :one
INSERT INTO habit_goals (habit_id, title, start_day, end_day, created_at)
VALUES (@habit_id, @title, @start_day, @end_day, @created_at)
RETURNING *;

-- name: UpdateGoal :one
UPDATE habit_goals SET habit_id = @habit_id, title = @title, start_day = @start_day, end_day = @end_day
WHERE id = @id
RETURNING *;

-- name: DeleteGoal :execrows
DELETE FROM habit_goals WHERE id = @id;

-- ---------- наблюдения ----------

-- name: ListNotes :many
-- Лента, новые сверху. Курсор — id последней показанной записи (before = 0 — с начала).
-- Фильтры необязательны: habit_id = 0 / kind = '' — все; general = true — только общие.
SELECT * FROM habit_notes
WHERE (@before::bigint = 0 OR id < @before::bigint)
  AND (@habit_id::bigint = 0 OR habit_id = @habit_id::bigint)
  AND (NOT @general::boolean OR habit_id IS NULL)
  AND (@kind::text = '' OR kind = @kind::text)
ORDER BY id DESC
LIMIT @lim;

-- name: GetNote :one
SELECT * FROM habit_notes WHERE id = @id;

-- name: CreateNote :one
INSERT INTO habit_notes (habit_id, kind, body, created_at, updated_at)
VALUES (@habit_id, @kind, @body, @now, @now)
RETURNING *;

-- name: UpdateNote :one
UPDATE habit_notes SET habit_id = @habit_id, kind = @kind, body = @body, updated_at = @now
WHERE id = @id
RETURNING *;

-- name: DeleteNote :execrows
DELETE FROM habit_notes WHERE id = @id;
