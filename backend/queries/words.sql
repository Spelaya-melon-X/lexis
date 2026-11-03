-- Запросы модуля words. Время («сейчас») всегда приходит параметром из platform/clock.

-- name: InsertWord :one
-- ON CONFLICT DO NOTHING: при дубликате (lower(term), lower(translation)) строки нет →
-- pgx.ErrNoRows, и сервис берёт id существующего слова через GetWordIDByKey.
INSERT INTO words (term, forms, translation, next_review_at, created_at)
VALUES (@term, @forms, @translation, @now, @now)
ON CONFLICT DO NOTHING
RETURNING id;

-- name: GetWordIDByKey :one
SELECT id FROM words
WHERE lower(term) = lower(@term) AND lower(translation) = lower(@translation);

-- name: GetWord :one
SELECT * FROM words WHERE id = $1;

-- name: ListWords :many
-- Фильтры необязательные: NULL → без фильтра.
SELECT w.* FROM words w
WHERE (sqlc.narg(level)::smallint IS NULL OR w.level = sqlc.narg(level))
  AND (sqlc.narg(group_id)::bigint IS NULL OR EXISTS (
        SELECT 1 FROM word_groups wg WHERE wg.word_id = w.id AND wg.group_id = sqlc.narg(group_id)))
ORDER BY w.created_at DESC, w.id DESC;

-- name: UpdateWordText :exec
UPDATE words SET term = @term, forms = @forms, translation = @translation
WHERE id = @id;

-- name: DeleteWord :execrows
DELETE FROM words WHERE id = $1;

-- name: UnarchiveWord :execrows
-- Архив → «Мастер», серия с нуля, due сразу (SPEC §6). Только для level = 5.
UPDATE words SET level = 4, streak = 0, next_review_at = @now
WHERE id = @id AND level = 5;

-- name: NextExamplePosition :one
SELECT COALESCE(MAX(position) + 1, 0)::smallint FROM word_examples WHERE word_id = $1;

-- name: InsertExample :execrows
-- 0 строк = такой пример у слова уже есть (UNIQUE (word_id, sentence)).
INSERT INTO word_examples (word_id, position, sentence)
VALUES (@word_id, @position, @sentence)
ON CONFLICT DO NOTHING;

-- name: DeleteExamples :exec
DELETE FROM word_examples WHERE word_id = $1;

-- name: ListExamplesByWordIDs :many
-- Одним запросом для всего списка слов, а не по запросу на слово (N+1).
SELECT word_id, sentence FROM word_examples
WHERE word_id = ANY(@word_ids::bigint[])
ORDER BY word_id, position;

-- name: InsertGroup :one
INSERT INTO groups (name, created_at) VALUES (@name, @now)
ON CONFLICT DO NOTHING
RETURNING id;

-- name: GetGroupIDByName :one
SELECT id FROM groups WHERE name = $1;

-- name: AddWordGroup :exec
INSERT INTO word_groups (word_id, group_id) VALUES (@word_id, @group_id)
ON CONFLICT DO NOTHING;

-- name: DeleteWordGroups :exec
DELETE FROM word_groups WHERE word_id = $1;

-- name: ListGroupsByWordIDs :many
SELECT wg.word_id, g.id, g.name FROM word_groups wg
JOIN groups g ON g.id = wg.group_id
WHERE wg.word_id = ANY(@word_ids::bigint[])
ORDER BY wg.word_id, g.name;
