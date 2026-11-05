-- Запросы модуля tasks (SPEC §18.3, фаза B1). Время — параметром из clock.

-- ---------- типы ----------

-- name: ListTypes :many
SELECT * FROM task_types ORDER BY position, id;

-- name: GetType :one
SELECT * FROM task_types WHERE id = @id;

-- name: TypeExists :one
SELECT EXISTS (SELECT 1 FROM task_types WHERE id = @id);

-- name: NextTypePosition :one
SELECT COALESCE(max(position), -1)::int + 1 FROM task_types;

-- name: CreateType :one
INSERT INTO task_types (name, color, position, created_at) VALUES (@name, @color, @position, @created_at)
RETURNING *;

-- name: UpdateType :one
UPDATE task_types SET name = @name, color = @color WHERE id = @id
RETURNING *;

-- name: SetTypePosition :exec
UPDATE task_types SET position = @position WHERE id = @id;

-- name: DeleteType :execrows
-- Дела этого типа остаются без типа (ON DELETE SET NULL).
DELETE FROM task_types WHERE id = @id;

-- ---------- колонки ----------

-- name: ListColumns :many
SELECT * FROM task_columns ORDER BY position, id;

-- name: GetColumnForUpdate :one
-- FOR UPDATE: перенос дел и удаление колонки не должны пересечься с добавлением дела в неё.
SELECT * FROM task_columns WHERE id = @id FOR UPDATE;

-- name: NextColumnPosition :one
SELECT COALESCE(max(position), -1)::int + 1 FROM task_columns;

-- name: CreateColumn :one
INSERT INTO task_columns (name, position, created_at) VALUES (@name, @position, @created_at)
RETURNING *;

-- name: RenameColumn :one
UPDATE task_columns SET name = @name WHERE id = @id
RETURNING *;

-- name: SetColumnPosition :exec
UPDATE task_columns SET position = @position WHERE id = @id;

-- name: CountColumnTasks :one
SELECT count(*)::int FROM tasks WHERE column_id = @column_id;

-- name: DeleteColumn :execrows
DELETE FROM task_columns WHERE id = @id;

-- ---------- дела ----------

-- name: ListTasks :many
-- Все дела: выполненность родителя считается по листьям (tasks.Rollup), поэтому фильтр
-- «выполнено / нет» применяет Go, а не SQL. Дел — сотни, грузим целиком.
SELECT * FROM tasks ORDER BY column_id, position, id;

-- name: GetTask :one
SELECT * FROM tasks WHERE id = @id;

-- name: GetTaskForUpdate :one
-- FOR UPDATE: два одновременных «разбить на шаги» одного дела не должны перемешать позиции.
SELECT * FROM tasks WHERE id = @id FOR UPDATE;

-- name: HasChildren :one
SELECT EXISTS (SELECT 1 FROM tasks WHERE parent_id = @id::bigint);

-- name: NextTaskPosition :one
SELECT COALESCE(max(position), -1)::int + 1 FROM tasks WHERE column_id = @column_id;

-- name: CreateTask :one
INSERT INTO tasks (parent_id, title, blocks, importance, deadline, type_id, column_id, position, note, created_at, updated_at)
VALUES (sqlc.narg(parent_id), @title, @blocks, @importance, @deadline, @type_id, @column_id, @position, @note, @now, @now)
RETURNING *;

-- name: UpdateTask :one
UPDATE tasks SET title = @title, blocks = @blocks, importance = @importance, deadline = @deadline,
    type_id = @type_id, note = @note, done_at = @done_at, updated_at = @now
WHERE id = @id
RETURNING *;

-- name: ColumnTaskIDs :many
-- Все дела колонки по порядку (и выполненные: у них тоже есть место в колонке).
SELECT id FROM tasks WHERE column_id = @column_id ORDER BY position, id;

-- name: PlaceTasks :exec
-- Раскладка колонки: ids[i] получает column_id и position = i (unnest WITH ORDINALITY — одним запросом).
UPDATE tasks t SET column_id = @column_id, position = o.pos - 1
FROM unnest(@ids::bigint[]) WITH ORDINALITY AS o(id, pos)
WHERE t.id = o.id;

-- name: MoveAllTasks :exec
-- При удалении колонки её дела уходят в конец другой, сохраняя свой порядок.
UPDATE tasks SET column_id = @to_column, position = position + @offset_by::int
WHERE column_id = @from_column;

-- name: DeleteTask :execrows
DELETE FROM tasks WHERE id = @id;

-- ---------- распорядок дня (B3) ----------

-- name: ListPlanItems :many
SELECT * FROM plan_items WHERE day = @day ORDER BY start_min, position;

-- name: ListPlanItemsRange :many
-- Пункты прошлых дней — для «не доделал».
SELECT * FROM plan_items WHERE day >= @from_day AND day < @to_day ORDER BY day DESC, position;

-- name: DeletePlan :exec
DELETE FROM plan_items WHERE day = @day;

-- name: InsertPlanItem :exec
INSERT INTO plan_items (day, task_id, position, blocks, start_min) VALUES (@day, @task_id, @position, @blocks, @start_min);

-- name: DeleteTaskPlans :exec
-- Дело разбили на шаги — само оно больше не планируется (в плане теперь место его шагам).
DELETE FROM plan_items WHERE task_id = @task_id;

-- ---------- настройки раздела ----------

-- name: GetSetting :one
SELECT value FROM settings WHERE key = @key;

-- name: PutSetting :exec
INSERT INTO settings (key, value) VALUES (@key, @value)
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value;

-- name: HasActiveFocus :one
-- Разбить дело, над которым идёт фокус-сессия, нельзя (§18.3, B4).
SELECT EXISTS (SELECT 1 FROM focus_sessions WHERE task_id = @task_id::bigint AND finished_at IS NULL);

-- name: TaskWorkSec :many
-- Чистое время в фокусе по делам (§18.3, B5): завершённые неотменённые сессии, минус перерывы.
SELECT s.task_id::bigint AS task_id,
       sum(GREATEST(0, extract(epoch FROM s.finished_at - s.work_started_at) - COALESCE(b.sec, 0)))::bigint AS work_sec
FROM focus_sessions s
LEFT JOIN (SELECT session_id, sum(extract(epoch FROM ended_at - started_at)) AS sec
           FROM focus_breaks WHERE ended_at IS NOT NULL GROUP BY session_id) b ON b.session_id = s.id
WHERE s.task_id IS NOT NULL AND s.finished_at IS NOT NULL AND s.work_started_at IS NOT NULL AND s.outcome <> 'abandoned'
GROUP BY s.task_id;
