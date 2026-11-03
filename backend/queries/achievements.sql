-- Запросы модуля achievements.

-- name: CountFinishedSessions :one
SELECT count(*)::int FROM training_sessions WHERE finished_at IS NOT NULL;

-- name: ListAllFinishedAt :many
SELECT finished_at::timestamptz AS finished_at FROM training_sessions
WHERE finished_at IS NOT NULL;

-- name: CountLearned :one
SELECT count(*)::int FROM words WHERE level >= 4;

-- name: CountArchivedGroups :one
-- Непустые группы, где все слова в «Архиве»: bool_and по пустому множеству дал бы NULL,
-- но JOIN оставляет только группы со словами.
SELECT count(*)::int FROM (
    SELECT wg.group_id
    FROM word_groups wg
    JOIN words w ON w.id = wg.word_id
    GROUP BY wg.group_id
    HAVING bool_and(w.level = 5)
) archived;

-- name: UnlockAchievement :execrows
-- 1 — разблокировано сейчас, 0 — уже было (идемпотентно: подписчик и GET могут прийти одновременно).
INSERT INTO achievements (code, unlocked_at) VALUES (@code, @now)
ON CONFLICT (code) DO NOTHING;

-- name: ListUnlocked :many
SELECT code, unlocked_at FROM achievements;
