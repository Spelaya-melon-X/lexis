-- Запросы модуля sport (SPEC §18.4). Время — параметром из clock.

-- ---------- тренировки ----------

-- name: ListWorkouts :many
SELECT * FROM workouts WHERE day BETWEEN @from_day AND @to_day ORDER BY day, created_at, id;

-- name: GetWorkout :one
SELECT * FROM workouts WHERE id = @id;

-- name: CreateWorkout :one
INSERT INTO workouts (day, title, notes, duration_min, kcal, created_at, updated_at)
VALUES (@day, @title, @notes, @duration_min, @kcal, @now, @now)
RETURNING *;

-- name: UpdateWorkout :one
UPDATE workouts SET day = @day, title = @title, notes = @notes, duration_min = @duration_min, kcal = @kcal, updated_at = @now
WHERE id = @id
RETURNING *;

-- name: DeleteWorkout :execrows
DELETE FROM workouts WHERE id = @id;

-- name: DeleteWorkoutExercises :exec
-- PUT заменяет упражнения целиком; подходы — каскадом.
DELETE FROM workout_exercises WHERE workout_id = @workout_id;

-- name: InsertWorkoutExercise :exec
INSERT INTO workout_exercises (workout_id, exercise_id, position, sets, reps, weight)
VALUES (@workout_id, @exercise_id, @position, @sets, @reps, @weight);

-- name: WorkoutExercisesOf :many
-- Упражнения тренировок (с названием и мерой из справочника) — одним запросом на весь список.
SELECT we.workout_id, we.exercise_id, we.sets, we.reps, we.weight, e.name
FROM workout_exercises we JOIN exercises e ON e.id = we.exercise_id
WHERE we.workout_id = ANY(@workout_ids::bigint[])
ORDER BY we.workout_id, we.position;

-- name: Titles :many
-- Подсказки названия тренировки: частые сверху, потом свежие.
SELECT title FROM workouts
WHERE @q::text = '' OR title ILIKE '%' || @q::text || '%'
GROUP BY title
ORDER BY count(*) DESC, max(day) DESC
LIMIT 10;

-- ---------- справочник упражнений ----------

-- name: ListExercises :many
-- С числом использований и последней датой — для справочника и подсказок.
SELECT e.*, count(we.id)::int AS uses, max(w.day) AS last_day
FROM exercises e
LEFT JOIN workout_exercises we ON we.exercise_id = e.id
LEFT JOIN workouts w ON w.id = we.workout_id
GROUP BY e.id
ORDER BY count(we.id) DESC, e.name;

-- name: GetExercise :one
SELECT * FROM exercises WHERE id = @id;

-- name: GetExerciseByName :one
SELECT * FROM exercises WHERE lower(name) = lower(@name);

-- name: CreateExercise :one
INSERT INTO exercises (name, created_at) VALUES (@name, @now)
RETURNING *;

-- name: RenameExercise :one
UPDATE exercises SET name = @name WHERE id = @id
RETURNING *;

-- name: DeleteExercise :execrows
DELETE FROM exercises WHERE id = @id;

-- name: ExerciseUsed :one
SELECT EXISTS (SELECT 1 FROM workout_exercises WHERE exercise_id = @id);

-- name: LastExerciseEntries :many
-- Последнее появление каждого упражнения (по дню, затем по созданию тренировки) — подставляется в форму
-- как значения по умолчанию («в прошлый раз 3 × 10 × 60 кг»).
SELECT DISTINCT ON (we.exercise_id) we.exercise_id, we.sets, we.reps, we.weight
FROM workout_exercises we JOIN workouts w ON w.id = we.workout_id
ORDER BY we.exercise_id, w.day DESC, w.created_at DESC, w.id DESC, we.position DESC;

-- ---------- еда (C2) ----------

-- name: ListMeals :many
SELECT * FROM meals WHERE day = @day ORDER BY position;

-- name: DeleteMeals :exec
DELETE FROM meals WHERE day = @day;

-- name: InsertMeal :exec
INSERT INTO meals (day, position, text, kcal, created_at) VALUES (@day, @position, @text, @kcal, @now);

-- name: MealSuggestions :many
-- Подсказки «что съел»: частые сверху; ккал — из самой свежей записи с таким текстом.
SELECT DISTINCT ON (lower(text)) text, kcal, count(*) OVER (PARTITION BY lower(text))::int AS uses
FROM meals
WHERE @q::text = '' OR text ILIKE '%' || @q::text || '%'
ORDER BY lower(text), day DESC, position DESC;

-- name: BurnedKcal :one
SELECT COALESCE(sum(kcal), 0)::int FROM workouts WHERE day = @day;

-- name: MealsBetween :many
SELECT day, kcal FROM meals WHERE day BETWEEN @from_day AND @to_day;

-- ---------- прогресс упражнения (C2) ----------

-- name: ExerciseHistory :many
-- Все записи упражнения в окне — по дням; прогресс по дням считает Go (sport.Progress).
SELECT w.day, we.sets, we.reps, we.weight
FROM workout_exercises we JOIN workouts w ON w.id = we.workout_id
WHERE we.exercise_id = @exercise_id AND w.day BETWEEN @from_day AND @to_day
ORDER BY w.day, w.id, we.position;
