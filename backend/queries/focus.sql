-- Запросы модуля focus (SPEC §18.3, B4). Время — параметром из clock.

-- ---------- сессии ----------

-- name: ActiveSession :one
SELECT * FROM focus_sessions WHERE finished_at IS NULL;

-- name: GetSession :one
SELECT * FROM focus_sessions WHERE id = @id;

-- name: GetSessionForUpdate :one
-- FOR UPDATE: «перерыв» и «я закончил» одной сессии не должны пересечься (двойной клик).
SELECT * FROM focus_sessions WHERE id = @id FOR UPDATE;

-- name: CreateSession :one
INSERT INTO focus_sessions (task_id, task_title, day, planned_blocks, started_at)
VALUES (@task_id, @task_title, @day, @planned_blocks, @now)
RETURNING *;

-- name: StartWork :exec
UPDATE focus_sessions SET work_started_at = @now::timestamptz WHERE id = @id;

-- name: FinishSession :exec
UPDATE focus_sessions SET finished_at = @now::timestamptz, outcome = @outcome::text WHERE id = @id;

-- name: PlannedBlocks :one
-- Сколько блоков дела стоит в плане на день — это и есть минимум работы в сессии.
SELECT blocks FROM plan_items WHERE day = @day AND task_id = @task_id;

-- ---------- перерывы и отвлечения ----------

-- name: ListBreaks :many
SELECT * FROM focus_breaks WHERE session_id = @session_id ORDER BY started_at, id;

-- name: StartBreak :exec
INSERT INTO focus_breaks (session_id, started_at) VALUES (@session_id, @now);

-- name: EndBreak :exec
UPDATE focus_breaks SET ended_at = @now::timestamptz WHERE session_id = @session_id AND ended_at IS NULL;

-- name: ListDistractions :many
SELECT * FROM focus_distractions WHERE session_id = @session_id ORDER BY at, id;

-- name: AddDistraction :exec
INSERT INTO focus_distractions (session_id, at, trigger, gave_in) VALUES (@session_id, @now, @trigger, @gave_in);

-- name: Triggers :many
-- Подсказки триггеров: частые сверху, потом свежие.
SELECT trigger, count(*)::int AS uses FROM focus_distractions
WHERE @q::text = '' OR trigger ILIKE '%' || @q::text || '%'
GROUP BY trigger
ORDER BY uses DESC, max(at) DESC
LIMIT 10;

-- ---------- ритуал ----------

-- name: ListRitual :many
SELECT * FROM ritual_items ORDER BY position, id;

-- name: DeleteRitual :exec
DELETE FROM ritual_items;

-- name: InsertRitual :exec
INSERT INTO ritual_items (text, position) VALUES (@text, @position);

-- ---------- награды и очки ----------

-- name: ListRewards :many
SELECT * FROM rewards ORDER BY position, id;

-- name: GetReward :one
SELECT * FROM rewards WHERE id = @id;

-- name: NextRewardPosition :one
SELECT COALESCE(max(position), -1)::int + 1 FROM rewards;

-- name: CreateReward :one
INSERT INTO rewards (title, price, position, created_at) VALUES (@title, @price, @position, @now)
RETURNING *;

-- name: UpdateReward :one
UPDATE rewards SET title = @title, price = @price, archived_at = @archived_at WHERE id = @id
RETURNING *;

-- name: SetRewardPosition :exec
UPDATE rewards SET position = @position WHERE id = @id;

-- name: DeleteReward :execrows
-- Покупки в журнале остаются (reward_id → NULL, название — снимком).
DELETE FROM rewards WHERE id = @id;

-- name: LockPoints :exec
-- Покупка читает баланс и списывает — под одной блокировкой, чтобы два клика не ушли в минус.
SELECT pg_advisory_xact_lock(@key::bigint);

-- name: Balance :one
SELECT COALESCE(sum(delta), 0)::int FROM points_ledger;

-- name: AddPoints :exec
INSERT INTO points_ledger (at, delta, kind, title, focus_session_id, reward_id)
VALUES (@at, @delta, @kind, @title, sqlc.narg(focus_session_id), sqlc.narg(reward_id));

-- name: Ledger :many
-- Лента, новые сверху; before — курсор (id последней показанной записи).
SELECT * FROM points_ledger
WHERE sqlc.narg(before)::bigint IS NULL OR id < sqlc.narg(before)::bigint
ORDER BY id DESC
LIMIT @lim;

-- name: SessionPoints :one
SELECT COALESCE(sum(delta), 0)::int FROM points_ledger WHERE focus_session_id = @session_id;

-- ---------- настройки ----------

-- name: GetSetting :one
SELECT value FROM settings WHERE key = @key;

-- name: PutSetting :exec
INSERT INTO settings (key, value) VALUES (@key, @value)
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value;

-- ---------- статистика (B5) ----------

-- name: StatSessions :many
-- Завершённые сессии окна (кроме отменённых: §18.3 — в статистику не идут).
SELECT * FROM focus_sessions
WHERE finished_at IS NOT NULL AND outcome <> 'abandoned' AND day BETWEEN @from_day AND @to_day
ORDER BY day, started_at, id;

-- name: BreaksOf :many
SELECT * FROM focus_breaks WHERE session_id = ANY(@ids::bigint[]) ORDER BY session_id, started_at, id;

-- name: DistractionsOf :many
SELECT * FROM focus_distractions WHERE session_id = ANY(@ids::bigint[]) ORDER BY session_id, at, id;

-- name: DoneTasksBetween :many
-- Выполненные дела-листья окна — и закрытые с доски, не только фокусом.
SELECT id, title, done_at::timestamptz AS done_at FROM tasks
WHERE done_at >= @from_at::timestamptz AND done_at < @to_at::timestamptz
ORDER BY done_at;

-- name: WorkByDay :many
-- Для рекордов за всё время: чистая работа по дням (та же формула, что у ачивок и времени дела).
SELECT s.day, sum(GREATEST(0, extract(epoch FROM s.finished_at - s.work_started_at) - COALESCE(b.sec, 0)))::bigint AS work_sec
FROM focus_sessions s
LEFT JOIN (SELECT session_id, sum(extract(epoch FROM ended_at - started_at)) AS sec
           FROM focus_breaks WHERE ended_at IS NOT NULL GROUP BY session_id) b ON b.session_id = s.id
WHERE s.finished_at IS NOT NULL AND s.work_started_at IS NOT NULL AND s.outcome <> 'abandoned'
GROUP BY s.day
ORDER BY s.day;

-- name: CountDoneTasks :one
SELECT count(*)::int FROM tasks WHERE done_at IS NOT NULL;
