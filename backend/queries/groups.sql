-- Запросы модуля groups. «Сегодня» — полуинтервал [day_start, day_end), посчитанный в Go.
-- Дерево групп (§18.2): всё «по группе» считается по поддереву — сама группа + потомки.

-- name: ListGroupSummaries :many
-- Одна строка на группу: агрегаты по поддереву (слова уникально — слово в двух подгруппах
-- считается один раз), свои слова, была ли сегодня сессия по самой группе или её предку.
-- sub(root, id) — замыкание «предок → потомок», включая пары (g, g). Групп — десятки,
-- поэтому строим его целиком даже для одной группы.
-- id NULL → все группы (для списка), иначе одна (для GET /:id).
WITH RECURSIVE sub (root, id) AS (
    SELECT g.id, g.id FROM groups g
    UNION ALL
    SELECT sub.root, c.id FROM sub JOIN groups c ON c.parent_id = sub.id
),
sub_words AS (
    SELECT DISTINCT sub.root, wg.word_id FROM sub JOIN word_groups wg ON wg.group_id = sub.id
)
SELECT g.id, g.name, g.color, g.parent_id, g.created_at,
       count(w.id)::int                                                          AS words,
       (SELECT count(*) FROM word_groups o WHERE o.group_id = g.id)::int         AS own_words,
       count(w.id) FILTER (WHERE w.level < 5 AND w.next_review_at < @day_end)::int AS due,
       count(w.id) FILTER (WHERE w.level = 1)::int                               AS level1,
       count(w.id) FILTER (WHERE w.level = 2)::int                               AS level2,
       count(w.id) FILTER (WHERE w.level = 3)::int                               AS level3,
       count(w.id) FILTER (WHERE w.level = 4)::int                               AS level4,
       count(w.id) FILTER (WHERE w.level = 5)::int                               AS level5,
       -- предки g — это root у пар (root, g): сессия предка тренировала и слова g
       EXISTS (SELECT 1 FROM training_sessions s
               WHERE s.group_id IN (SELECT a.root FROM sub a WHERE a.id = g.id)
                 AND s.finished_at >= @day_start::timestamptz AND s.finished_at < @day_end)::bool AS trained_today
FROM groups g
LEFT JOIN sub_words sw ON sw.root = g.id
LEFT JOIN words w ON w.id = sw.word_id
WHERE sqlc.narg(id)::bigint IS NULL OR g.id = sqlc.narg(id)
GROUP BY g.id
ORDER BY g.name;

-- name: CountFinishedSessions :one
-- Сессии по группе и её потомкам.
WITH RECURSIVE sub (id) AS (
    SELECT @group_id::bigint
    UNION ALL
    SELECT c.id FROM groups c JOIN sub ON c.parent_id = sub.id
)
SELECT count(*)::int FROM training_sessions
WHERE group_id IN (SELECT id FROM sub) AND finished_at >= @from_at::timestamptz AND finished_at < @to_at::timestamptz;

-- name: ListFinishedSessions :many
-- Для графика продуктивности (по поддереву); по дням раскладывает groups.Productivity в TZ пользователя.
WITH RECURSIVE sub (id) AS (
    SELECT @group_id::bigint
    UNION ALL
    SELECT c.id FROM groups c JOIN sub ON c.parent_id = sub.id
)
SELECT finished_at::timestamptz AS finished_at, total, good FROM training_sessions
WHERE group_id IN (SELECT id FROM sub) AND finished_at >= @from_at::timestamptz AND finished_at < @to_at::timestamptz
ORDER BY finished_at;

-- name: LockTree :exec
-- Все изменения дерева — под одной транзакционной блокировкой (см. tree.LockKey).
SELECT pg_advisory_xact_lock(@key::bigint);

-- name: ListGroupNodes :many
SELECT id, name, parent_id FROM groups;

-- name: InsertGroup :one
INSERT INTO groups (name, parent_id, color, created_at) VALUES (@name, sqlc.narg(parent_id), sqlc.narg(color), @now)
RETURNING id;

-- name: UpdateGroup :execrows
UPDATE groups SET name = @name, color = @color, parent_id = @parent_id WHERE id = @id;

-- name: GetGroup :one
SELECT * FROM groups WHERE id = $1;

-- name: LiftChildren :exec
-- Подгруппы удаляемой группы поднимаются на её уровень.
UPDATE groups SET parent_id = sqlc.narg(new_parent) WHERE parent_id = @id::bigint;

-- name: DeleteGroups :execrows
-- word_groups — каскадом, training_sessions.group_id → NULL (история сохраняется).
-- Удаляем всё поддерево одним запросом: RESTRICT на parent_id проверяется в конце оператора,
-- когда детей уже нет.
DELETE FROM groups WHERE id = ANY(@ids::bigint[]);
