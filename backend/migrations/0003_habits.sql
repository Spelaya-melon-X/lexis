-- +goose Up

-- Раздел «Привычки» (SPEC §17.4). Дни — DATE в TZ пользователя (считает Go через clock),
-- время — параметром из clock, без DEFAULT now().

CREATE TABLE habits (
    id            BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name          TEXT        NOT NULL CHECK (name <> '' AND char_length(name) <= 60),
    kind          TEXT        NOT NULL CHECK (kind IN ('positive', 'negative')),
    color         TEXT        CHECK (color ~ '^#[0-9a-f]{6}$'),
    -- Опциональные поля отметки: что спрашивать при отметке за день.
    track_trigger BOOLEAN     NOT NULL DEFAULT false,
    track_rating  BOOLEAN     NOT NULL DEFAULT false,
    track_amount  BOOLEAN     NOT NULL DEFAULT false,
    unit          TEXT        NOT NULL DEFAULT '' CHECK (char_length(unit) <= 20),  -- «км», «стаканов»
    position      INT         NOT NULL,           -- порядок в списках
    archived_at   TIMESTAMPTZ,                    -- в архиве: нет в отметках, история остаётся
    created_at    TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX habits_name_uq ON habits (lower(name));

-- Не больше одной отметки на привычку в день (Q5): «бегал дважды» — сумма в amount.
-- Для негативной привычки отметка = «сорвался» (Q4).
CREATE TABLE habit_checkins (
    habit_id   BIGINT           NOT NULL REFERENCES habits (id) ON DELETE CASCADE,
    day        DATE             NOT NULL,
    trigger    TEXT             CHECK (trigger <> '' AND char_length(trigger) <= 200),
    rating     SMALLINT         CHECK (rating BETWEEN 1 AND 10),
    -- DOUBLE PRECISION, а не NUMERIC: pgx отдаёт NUMERIC как pgtype.Numeric, а точности
    -- float64 для «5.2 км» хватает с запасом (фронт округляет до 2 знаков).
    amount     DOUBLE PRECISION CHECK (amount >= 0),
    created_at TIMESTAMPTZ      NOT NULL,
    updated_at TIMESTAMPTZ      NOT NULL,
    PRIMARY KEY (habit_id, day)
);
-- «все отметки за период» (календарь, страница дня); PK покрывает выборку по привычке.
CREATE INDEX habit_checkins_day_idx ON habit_checkins (day);

-- Цель на период — напоминание, не план (§17.4). Без привычки — свободный текст.
CREATE TABLE habit_goals (
    id         BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    habit_id   BIGINT      REFERENCES habits (id) ON DELETE CASCADE,
    title      TEXT        NOT NULL DEFAULT '' CHECK (char_length(title) <= 200),
    start_day  DATE        NOT NULL,
    end_day    DATE        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    CHECK (end_day >= start_day),
    CHECK (habit_id IS NOT NULL OR title <> '')
);
-- «цели, пересекающие окно»: end_day >= from AND start_day <= to.
CREATE INDEX habit_goals_range_idx ON habit_goals (end_day, start_day);

-- Наблюдения и выводы: лента, новые сверху (по id — порядок создания).
-- Удаление привычки не стирает выводы: они становятся общими (SET NULL).
CREATE TABLE habit_notes (
    id         BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    habit_id   BIGINT      REFERENCES habits (id) ON DELETE SET NULL,
    kind       TEXT        NOT NULL CHECK (kind IN ('observation', 'insight')),
    body       TEXT        NOT NULL CHECK (body <> '' AND char_length(body) <= 20000),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX habit_notes_habit_idx ON habit_notes (habit_id, id);

-- +goose Down
DROP TABLE habit_notes;
DROP TABLE habit_goals;
DROP TABLE habit_checkins;
DROP TABLE habits;
