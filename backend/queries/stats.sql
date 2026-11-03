-- Запросы модуля stats (только чтение). Окна [from, to) считаются в Go в TZ пользователя.

-- name: ListFinishedSessions :many
-- Сессии, завершённые в окне; по дням раскладывает stats.BucketByDay.
SELECT finished_at::timestamptz AS finished_at, group_id, total, good, hard, again, duration_sec
FROM training_sessions
WHERE finished_at >= @from_at::timestamptz AND finished_at < @to_at::timestamptz
ORDER BY finished_at;

-- name: ListAllFinishedAt :many
-- Для серии дней нужна вся история; строк немного (единицы сессий в день).
SELECT finished_at::timestamptz AS finished_at FROM training_sessions
WHERE finished_at IS NOT NULL;

-- name: WordsByGroup :many
-- Уникальные разобранные слова за окно по ВСЕМ группам слова (§8 «по группам-типам»):
-- слово с тегами #глаголы #спорт считается в обеих.
SELECT g.id, g.name, count(DISTINCT r.word_id)::int AS words
FROM reviews r
JOIN training_sessions s ON s.id = r.session_id
JOIN word_groups wg ON wg.word_id = r.word_id
JOIN groups g ON g.id = wg.group_id
WHERE r.first_in_session
  AND s.finished_at >= @from_at::timestamptz AND s.finished_at < @to_at::timestamptz
GROUP BY g.id
ORDER BY words DESC, g.name;

-- name: LevelTotals :one
SELECT count(*) FILTER (WHERE level = 1)::int AS level1,
       count(*) FILTER (WHERE level = 2)::int AS level2,
       count(*) FILTER (WHERE level = 3)::int AS level3,
       count(*) FILTER (WHERE level = 4)::int AS level4,
       count(*) FILTER (WHERE level = 5)::int AS level5
FROM words;
