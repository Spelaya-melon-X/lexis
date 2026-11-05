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

-- «Игра» (§18.3, B5).

-- name: CountDoneTasks :one
-- done_at бывает только у листьев (выполненность родителя вычисляется), так что это «закрытые дела-шаги».
SELECT count(*)::int FROM tasks WHERE done_at IS NOT NULL;

-- name: FocusWorkSec :one
-- Чистая работа во всех завершённых сессиях (кроме отменённых): от «начал делать» до конца минус перерывы.
-- Перерывы завершённой сессии всегда закрыты (Finish закрывает открытый) — их длина известна.
SELECT COALESCE(sum(GREATEST(0, extract(epoch FROM s.finished_at - s.work_started_at) - COALESCE(b.sec, 0))), 0)::bigint
FROM focus_sessions s
LEFT JOIN (SELECT session_id, sum(extract(epoch FROM ended_at - started_at)) AS sec
           FROM focus_breaks WHERE ended_at IS NOT NULL GROUP BY session_id) b ON b.session_id = s.id
WHERE s.finished_at IS NOT NULL AND s.work_started_at IS NOT NULL AND s.outcome <> 'abandoned';
