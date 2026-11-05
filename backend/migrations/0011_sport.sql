-- +goose Up

-- Раздел «Спорт», фаза C1 (SPEC §18.4): тренировки, справочник упражнений, подходы.
-- Время — параметром из clock; день тренировки — DATE в TZ пользователя.

-- Справочник упражнений. measure — что меряем в подходе (§18.4 C2):
--   weight — повторы × вес (оба обязательны); reps — повторы (вес — необязательный «доп. вес»);
--   time   — секунды (вес необязателен: планка с блином).
CREATE TABLE exercises (
    id         BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name       TEXT        NOT NULL CHECK (name <> '' AND char_length(name) <= 60),
    measure    TEXT        NOT NULL CHECK (measure IN ('weight', 'reps', 'time')),
    created_at TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX exercises_name_uq ON exercises (lower(name));

CREATE TABLE workouts (
    id           BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    day          DATE        NOT NULL,
    title        TEXT        NOT NULL CHECK (title <> '' AND char_length(title) <= 100),
    notes        TEXT        NOT NULL DEFAULT '' CHECK (char_length(notes) <= 20000),  -- «как прошла»
    duration_min INT         CHECK (duration_min BETWEEN 1 AND 1440),                   -- с Apple Watch, вручную
    kcal         INT         CHECK (kcal BETWEEN 0 AND 20000),
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL
);
CREATE INDEX workouts_day_idx ON workouts (day);

-- Тренировка сохраняется целиком (PUT заменяет упражнения и подходы), поэтому position — просто порядок.
CREATE TABLE workout_exercises (
    id          BIGINT   GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    workout_id  BIGINT   NOT NULL REFERENCES workouts (id) ON DELETE CASCADE,
    -- RESTRICT: упражнение из справочника, которое где-то записано, удалить нельзя (409 exercise_in_use).
    exercise_id BIGINT   NOT NULL REFERENCES exercises (id) ON DELETE RESTRICT,
    position    SMALLINT NOT NULL,
    UNIQUE (workout_id, position)
);
-- «Подходы из прошлого раза» и прогресс упражнения.
CREATE INDEX workout_exercises_exercise_idx ON workout_exercises (exercise_id);

-- Какие поля заполнены, зависит от measure упражнения — это проверяет Go (sport.ValidateSet).
CREATE TABLE workout_sets (
    workout_exercise_id BIGINT           NOT NULL REFERENCES workout_exercises (id) ON DELETE CASCADE,
    position            SMALLINT         NOT NULL,
    reps                INT              CHECK (reps BETWEEN 1 AND 10000),
    weight              DOUBLE PRECISION CHECK (weight BETWEEN 0 AND 1000),   -- кг; DOUBLE — как amount у привычек
    seconds             INT              CHECK (seconds BETWEEN 1 AND 86400),
    PRIMARY KEY (workout_exercise_id, position)
);

-- +goose Down
DROP TABLE workout_sets;
DROP TABLE workout_exercises;
DROP TABLE workouts;
DROP TABLE exercises;
