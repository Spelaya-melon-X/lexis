-- +goose Up

-- «Спорт», доработка C1 по отзыву (SPEC §18.5.7): упражнение в тренировке — одна строка
-- «подходы × повторы × вес», без отдельных строк на каждый подход и без упражнений на время.
ALTER TABLE workout_exercises
    ADD COLUMN sets   SMALLINT,
    ADD COLUMN reps   INT,
    ADD COLUMN weight DOUBLE PRECISION;

-- Перенос записанного (на момент миграции данных нет, но правило общее): число подходов,
-- повторы и вес первого подхода; у подхода «на время» повторов нет — считаем 1.
UPDATE workout_exercises we SET
    sets   = GREATEST(1, (SELECT count(*) FROM workout_sets s WHERE s.workout_exercise_id = we.id)),
    reps   = COALESCE((SELECT s.reps FROM workout_sets s WHERE s.workout_exercise_id = we.id ORDER BY s.position LIMIT 1), 1),
    weight = (SELECT s.weight FROM workout_sets s WHERE s.workout_exercise_id = we.id ORDER BY s.position LIMIT 1);

ALTER TABLE workout_exercises
    ALTER COLUMN sets SET NOT NULL,
    ALTER COLUMN reps SET NOT NULL,
    ADD CONSTRAINT workout_exercises_sets_check   CHECK (sets BETWEEN 1 AND 50),
    ADD CONSTRAINT workout_exercises_reps_check   CHECK (reps BETWEEN 1 AND 10000),
    ADD CONSTRAINT workout_exercises_weight_check CHECK (weight BETWEEN 0 AND 1000);  -- NULL — без веса (подтягивания)

DROP TABLE workout_sets;
ALTER TABLE exercises DROP COLUMN measure;

-- +goose Down
ALTER TABLE exercises ADD COLUMN measure TEXT NOT NULL DEFAULT 'weight' CHECK (measure IN ('weight', 'reps', 'time'));
UPDATE exercises e SET measure = 'reps'
WHERE NOT EXISTS (SELECT 1 FROM workout_exercises we WHERE we.exercise_id = e.id AND we.weight IS NOT NULL);
CREATE TABLE workout_sets (
    workout_exercise_id BIGINT           NOT NULL REFERENCES workout_exercises (id) ON DELETE CASCADE,
    position            SMALLINT         NOT NULL,
    reps                INT              CHECK (reps BETWEEN 1 AND 10000),
    weight              DOUBLE PRECISION CHECK (weight BETWEEN 0 AND 1000),
    seconds             INT              CHECK (seconds BETWEEN 1 AND 86400),
    PRIMARY KEY (workout_exercise_id, position)
);
INSERT INTO workout_sets (workout_exercise_id, position, reps, weight)
SELECT we.id, g.n, we.reps, we.weight FROM workout_exercises we, generate_series(0, we.sets - 1) AS g(n);
ALTER TABLE workout_exercises DROP COLUMN sets, DROP COLUMN reps, DROP COLUMN weight;
