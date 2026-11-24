-- name: LockStudy :exec
SELECT pg_advisory_xact_lock(741925);
-- name: NextID :one
SELECT nextval('study_ids')::bigint;
-- name: AdvanceIDs :exec
SELECT setval('study_ids', greatest(@max_id::bigint, coalesce(pg_sequence_last_value('study_ids'::regclass), 1)), true);
-- name: ListGroups :many
SELECT * FROM study_groups ORDER BY title, id;
-- name: ListCards :many
SELECT * FROM study_cards ORDER BY id;
-- name: SaveGroup :exec
INSERT INTO study_groups (id, title, parent_id, grammar) VALUES (@id, @title, sqlc.narg(parent_id), @grammar)
ON CONFLICT (id) DO UPDATE SET title=EXCLUDED.title, parent_id=EXCLUDED.parent_id, grammar=EXCLUDED.grammar;
-- name: SaveCard :exec
INSERT INTO study_cards (id, group_id, kind, front, back, grammar_content) VALUES (@id, @group_id, @kind, @front, @back, sqlc.narg(grammar_content))
ON CONFLICT (id) DO UPDATE SET group_id=EXCLUDED.group_id, kind=EXCLUDED.kind, front=EXCLUDED.front, back=EXCLUDED.back, grammar_content=EXCLUDED.grammar_content;
-- name: PruneCards :exec
DELETE FROM study_cards WHERE NOT (id = ANY(@ids::bigint[]));
-- name: PruneGroups :exec
DELETE FROM study_groups WHERE NOT (id = ANY(@ids::bigint[]));

-- name: InsertReview :execrows
INSERT INTO study_reviews (id, card_id, reviewed_at, known, reverse)
VALUES (@id, @card_id, @reviewed_at, @known, @reverse) ON CONFLICT (id) DO NOTHING;
-- name: GetReview :one
SELECT * FROM study_reviews WHERE id = $1;
-- name: ListReviews :many
SELECT * FROM study_reviews ORDER BY reviewed_at, id;
-- name: ListReviewsInRange :many
SELECT * FROM study_reviews WHERE reviewed_at >= @from_at::timestamptz AND reviewed_at < @to_at::timestamptz ORDER BY reviewed_at, id;
-- name: ClearReviews :exec
DELETE FROM study_reviews;
-- name: IsGrammarCard :one
SELECT EXISTS (SELECT 1 FROM study_cards WHERE id = $1 AND kind = 'grammar_rule');

-- name: ListKnowledge :many
WITH scores AS (
 SELECT card_id, sum(CASE WHEN known THEN 1 ELSE -1 END)::bigint AS score,
 count(*)::bigint AS total, count(*) FILTER (WHERE known)::bigint AS known,
 max(reviewed_at) AS last_reviewed_at
 FROM study_reviews GROUP BY card_id
)
SELECT c.id AS card_id, c.group_id, c.front, c.kind, g.title AS group_title,
 coalesce(s.score, 0)::bigint AS score, coalesce(s.total, 0)::bigint AS total,
 coalesce(s.known, 0)::bigint AS known, coalesce(s.last_reviewed_at, '0001-01-01T00:00:00Z'::timestamptz)::timestamptz AS last_reviewed_at
FROM study_cards c JOIN study_groups g ON g.id=c.group_id
LEFT JOIN scores s ON s.card_id=c.id WHERE c.kind <> 'grammar_rule' ORDER BY c.id;
