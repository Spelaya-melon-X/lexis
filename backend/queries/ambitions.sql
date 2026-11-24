-- name: LockAmbitions :exec
SELECT pg_advisory_xact_lock(741924);

-- name: NextID :one
SELECT nextval('ambition_ids')::bigint;

-- name: AdvanceIDs :exec
SELECT setval('ambition_ids', greatest(@max_id::bigint, coalesce(pg_sequence_last_value('ambition_ids'::regclass), 1)), true);

-- name: ListCategories :many
SELECT * FROM dream_categories ORDER BY position, id;

-- name: SaveCategory :exec
INSERT INTO dream_categories (id, title, position) VALUES (@id, @title, @position)
ON CONFLICT (id) DO UPDATE SET title=EXCLUDED.title, position=EXCLUDED.position;

-- name: PruneCategories :exec
DELETE FROM dream_categories WHERE NOT (id = ANY(@ids::bigint[]));

-- name: ListDreams :many
SELECT * FROM dreams ORDER BY position, id;

-- name: SaveDream :exec
INSERT INTO dreams (id, title, note, deadline, status, position, category_id, project_id) VALUES (@id, @title, @note, sqlc.narg(deadline), @status, @position, sqlc.narg(category_id), sqlc.narg(project_id))
ON CONFLICT (id) DO UPDATE SET title=EXCLUDED.title, note=EXCLUDED.note, deadline=EXCLUDED.deadline, status=EXCLUDED.status, position=EXCLUDED.position, category_id=EXCLUDED.category_id, project_id=EXCLUDED.project_id;

-- name: PruneDreams :exec
DELETE FROM dreams WHERE NOT (id = ANY(@ids::bigint[]));

-- name: ListProjects :many
SELECT * FROM big_projects ORDER BY position, id;

-- name: SaveProject :exec
INSERT INTO big_projects (id, title, note, deadline, position) VALUES (@id, @title, @note, sqlc.narg(deadline), @position)
ON CONFLICT (id) DO UPDATE SET title=EXCLUDED.title, note=EXCLUDED.note, deadline=EXCLUDED.deadline, position=EXCLUDED.position;

-- name: PruneProjects :exec
DELETE FROM big_projects WHERE NOT (id = ANY(@ids::bigint[]));

-- name: ListSteps :many
SELECT * FROM project_steps ORDER BY position, id;

-- name: SaveStep :exec
INSERT INTO project_steps (id, project_id, parent_id, title, note, deadline, position, done) VALUES (@id, @project_id, sqlc.narg(parent_id), @title, @note, sqlc.narg(deadline), @position, @done)
ON CONFLICT (id) DO UPDATE SET project_id=EXCLUDED.project_id, parent_id=EXCLUDED.parent_id, title=EXCLUDED.title, note=EXCLUDED.note, deadline=EXCLUDED.deadline, position=EXCLUDED.position, done=EXCLUDED.done;

-- name: PruneSteps :exec
DELETE FROM project_steps WHERE NOT (id = ANY(@ids::bigint[]));

-- name: ListDependencies :many
SELECT * FROM step_dependencies ORDER BY step_id, requires_id;

-- name: ClearDependencies :exec
DELETE FROM step_dependencies;

-- name: SaveDependency :exec
INSERT INTO step_dependencies (project_id, step_id, requires_id) VALUES (@project_id, @step_id, @requires_id);

-- name: ListTimelineYears :many
SELECT year FROM timeline_years ORDER BY year;
-- name: SaveTimelineYear :exec
INSERT INTO timeline_years (year) VALUES ($1) ON CONFLICT DO NOTHING;
-- name: PruneTimelineYears :exec
DELETE FROM timeline_years WHERE NOT (year = ANY(@years::integer[]));
-- name: ListTimelineGoals :many
SELECT * FROM timeline_goals ORDER BY year, month NULLS FIRST, id;
-- name: SaveTimelineGoal :exec
INSERT INTO timeline_goals (id, year, month, title, note, dream_id)
VALUES (@id, @year, sqlc.narg(month), @title, @note, sqlc.narg(dream_id))
ON CONFLICT (id) DO UPDATE SET year=EXCLUDED.year, month=EXCLUDED.month, title=EXCLUDED.title, note=EXCLUDED.note, dream_id=EXCLUDED.dream_id;
-- name: PruneTimelineGoals :exec
DELETE FROM timeline_goals WHERE NOT (id = ANY(@ids::bigint[]));
