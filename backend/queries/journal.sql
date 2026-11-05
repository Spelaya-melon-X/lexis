-- Запросы модуля journal: раздел «День» (SPEC §17.2).

-- name: GetDay :one
SELECT * FROM days WHERE day = @day;

-- name: EnsureDay :exec
-- Строка дня создаётся лениво — при первой записи; повторный вызов только двигает updated_at.
INSERT INTO days (day, created_at, updated_at) VALUES (@day, @now, @now)
ON CONFLICT (day) DO UPDATE SET updated_at = EXCLUDED.updated_at;

-- name: SetRating :exec
UPDATE days SET rating = @rating WHERE day = @day;

-- name: SetStory :exec
UPDATE days SET story = @story WHERE day = @day;

-- name: ListWins :many
SELECT text FROM day_wins WHERE day = @day ORDER BY position;

-- name: DeleteWins :exec
DELETE FROM day_wins WHERE day = @day;

-- name: InsertWin :exec
INSERT INTO day_wins (day, position, text) VALUES (@day, @position, @text);

-- name: ListDopamine :many
SELECT slot, text FROM dopamine_slots WHERE day = @day ORDER BY slot;

-- name: UpsertDopamine :exec
INSERT INTO dopamine_slots (day, slot, text, updated_at) VALUES (@day, @slot, @text, @now)
ON CONFLICT (day, slot) DO UPDATE SET text = EXCLUDED.text, updated_at = EXCLUDED.updated_at;

-- name: DeleteDopamine :exec
DELETE FROM dopamine_slots WHERE day = @day AND slot = @slot;

-- name: ListDaySummaries :many
-- Сводка по дням окна для календаря и графиков. Дни без строки в days сюда не попадают —
-- пустые дни дорисовывает Go (journal.FillDays).
SELECT d.day, d.rating, d.story <> '' AS has_story,
       (SELECT count(*) FROM dopamine_slots s WHERE s.day = d.day)::int AS dopamine,
       (SELECT count(*) FROM day_wins w WHERE w.day = d.day)::int AS wins
FROM days d
WHERE d.day BETWEEN @from_day AND @to_day
ORDER BY d.day;

-- ---------- недельный анализ (§17.3) ----------

-- name: GetWeek :one
SELECT * FROM week_reviews WHERE week_start = @week_start;

-- name: ListWeekBlocks :many
SELECT topic, conclusion FROM week_review_blocks WHERE week_start = @week_start ORDER BY position;

-- name: UpsertWeek :exec
INSERT INTO week_reviews (week_start, created_at, updated_at) VALUES (@week_start, @now, @now)
ON CONFLICT (week_start) DO UPDATE SET updated_at = EXCLUDED.updated_at;

-- name: DeleteWeek :exec
-- Пустой анализ не храним: строка недели уходит вместе с блоками (CASCADE).
DELETE FROM week_reviews WHERE week_start = @week_start;

-- name: DeleteWeekBlocks :exec
DELETE FROM week_review_blocks WHERE week_start = @week_start;

-- name: InsertWeekBlock :exec
INSERT INTO week_review_blocks (week_start, position, topic, conclusion) VALUES (@week_start, @position, @topic, @conclusion);
