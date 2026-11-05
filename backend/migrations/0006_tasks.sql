-- +goose Up

-- Раздел «Игра в жизнь», фаза B1 (SPEC §18.3): типы дел, колонки доски, дела.
-- Время — параметром из clock, без DEFAULT now(). Подзадачи (parent_id) — в 0007, план дня — в 0008.

-- Тип дела (учёба, жизнь…) — свой список пользователя с цветом и порядком.
CREATE TABLE task_types (
    id         BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name       TEXT        NOT NULL CHECK (name <> '' AND char_length(name) <= 40),
    color      TEXT        CHECK (color ~ '^#[0-9a-f]{6}$'),
    position   INT         NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX task_types_name_uq ON task_types (lower(name));

-- Колонка доски — пользовательский список. Первую создаёт пользователь (название зависит от языка).
CREATE TABLE task_columns (
    id         BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name       TEXT        NOT NULL CHECK (name <> '' AND char_length(name) <= 40),
    position   INT         NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE tasks (
    id         BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title      TEXT        NOT NULL CHECK (title <> '' AND char_length(title) <= 200),
    blocks     SMALLINT    NOT NULL CHECK (blocks BETWEEN 1 AND 40),       -- объём в блоках по 30 минут
    importance SMALLINT    NOT NULL CHECK (importance BETWEEN 1 AND 10),
    deadline   DATE,
    -- Удаление типа не трогает дела — они просто остаются без типа.
    type_id    BIGINT      REFERENCES task_types (id) ON DELETE SET NULL,
    -- RESTRICT: колонку с делами удаляет сервис, сначала перенеся дела в другую (§18.3).
    column_id  BIGINT      NOT NULL REFERENCES task_columns (id) ON DELETE RESTRICT,
    position   INT         NOT NULL,                                       -- порядок внутри колонки
    note       TEXT        NOT NULL DEFAULT '' CHECK (char_length(note) <= 20000),
    done_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
-- Доска: дела колонки по порядку.
CREATE INDEX tasks_column_idx ON tasks (column_id, position);
CREATE INDEX tasks_type_idx ON tasks (type_id);

-- +goose Down
DROP TABLE tasks;
DROP TABLE task_columns;
DROP TABLE task_types;
