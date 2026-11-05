-- +goose Up

-- Раздел «День» (SPEC §17.2). День — DATE в TZ пользователя (дату считает Go через clock),
-- строка создаётся при первой записи за этот день. Время — параметром из clock, без DEFAULT now().

CREATE TABLE days (
    day        DATE        PRIMARY KEY,
    rating     SMALLINT    CHECK (rating BETWEEN 1 AND 10),  -- NULL — день не оценён
    story      TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

-- «Что классного сделал» — упорядоченный список, сохраняется целиком (PATCH заменяет все пункты).
CREATE TABLE day_wins (
    id       BIGINT   GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    day      DATE     NOT NULL REFERENCES days (day) ON DELETE CASCADE,
    position SMALLINT NOT NULL,
    text     TEXT     NOT NULL CHECK (text <> ''),
    UNIQUE (day, position)
);

-- Дофаминовая шкала: 7 квадратиков на день, пустой квадратик = нет строки,
-- поэтому уровень дня = count(*). Длину в символах (а не байтах) проверяет char_length.
CREATE TABLE dopamine_slots (
    day        DATE        NOT NULL REFERENCES days (day) ON DELETE CASCADE,
    slot       SMALLINT    NOT NULL CHECK (slot BETWEEN 0 AND 6),
    text       TEXT        NOT NULL CHECK (text <> '' AND char_length(text) <= 140),
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (day, slot)
);

-- +goose Down
DROP TABLE dopamine_slots;
DROP TABLE day_wins;
DROP TABLE days;
