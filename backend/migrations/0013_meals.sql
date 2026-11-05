-- +goose Up

-- «Спорт», фаза C2 (SPEC §18.4): еда за день — список «что съел + примерные ккал».
-- Сохраняется целиком (PUT заменяет список, как «Что классного» в дневнике), поэтому position — просто порядок.
CREATE TABLE meals (
    id         BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    day        DATE        NOT NULL,
    position   SMALLINT    NOT NULL,
    text       TEXT        NOT NULL CHECK (text <> '' AND char_length(text) <= 200),
    kcal       INT         CHECK (kcal BETWEEN 0 AND 10000),  -- NULL — не знаю сколько
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (day, position)
);

-- +goose Down
DROP TABLE meals;
