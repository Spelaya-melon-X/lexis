-- Запросы модуля groups. «Сегодня» — полуинтервал [day_start, day_end), посчитанный в Go.

-- name: ListGroupSummaries :many
-- Одна строка на группу: число слов, due, распределение по уровням и была ли сегодня
-- завершённая сессия. LEFT JOIN — чтобы пустые группы тоже попали (с нулями).
-- id NULL → все группы (для списка), иначе одна (для GET /:id).
SELECT g.id, g.name, g.color, g.created_at,
       count(w.id)::int                                                          AS words,
       count(w.id) FILTER (WHERE w.level < 5 AND w.next_review_at < @day_end)::int AS due,
       count(w.id) FILTER (WHERE w.level = 1)::int                               AS level1,
       count(w.id) FILTER (WHERE w.level = 2)::int                               AS level2,
       count(w.id) FILTER (WHERE w.level = 3)::int                               AS level3,
       count(w.id) FILTER (WHERE w.level = 4)::int                               AS level4,
       count(w.id) FILTER (WHERE w.level = 5)::int                               AS level5,
       EXISTS (SELECT 1 FROM training_sessions s
               WHERE s.group_id = g.id
                 AND s.finished_at >= @day_start::timestamptz AND s.finished_at < @day_end)::bool AS trained_today
FROM groups g
LEFT JOIN word_groups wg ON wg.group_id = g.id
LEFT JOIN words w ON w.id = wg.word_id
WHERE sqlc.narg(id)::bigint IS NULL OR g.id = sqlc.narg(id)
GROUP BY g.id
ORDER BY g.name;

-- name: CountFinishedSessions :one
SELECT count(*)::int FROM training_sessions
WHERE group_id = @group_id::bigint AND finished_at >= @from_at::timestamptz AND finished_at < @to_at::timestamptz;

-- name: ListFinishedSessions :many
-- Для графика продуктивности; по дням раскладывает groups.Productivity в TZ пользователя.
SELECT finished_at::timestamptz AS finished_at, total, good FROM training_sessions
WHERE group_id = @group_id::bigint AND finished_at >= @from_at::timestamptz AND finished_at < @to_at::timestamptz
ORDER BY finished_at;

-- name: UpdateGroup :execrows
UPDATE groups SET name = @name, color = @color WHERE id = @id;

-- name: GetGroup :one
SELECT * FROM groups WHERE id = $1;

-- name: DeleteGroup :execrows
-- word_groups — каскадом, training_sessions.group_id → NULL (история сохраняется).
DELETE FROM groups WHERE id = $1;
