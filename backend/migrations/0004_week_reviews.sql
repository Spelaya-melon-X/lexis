-- +goose Up

-- Недельный анализ (SPEC §17.3): запись недели = упорядоченные блоки «о чём / вывод».
-- Неделя — с понедельника; ключ — дата понедельника (DATE в TZ пользователя, считает Go).

CREATE TABLE week_reviews (
    week_start DATE        PRIMARY KEY CHECK (extract(isodow FROM week_start) = 1),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

-- Сохраняется целиком (PUT заменяет все блоки), поэтому позиция — просто порядок.
CREATE TABLE week_review_blocks (
    id         BIGINT   GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    week_start DATE     NOT NULL REFERENCES week_reviews (week_start) ON DELETE CASCADE,
    position   SMALLINT NOT NULL,
    topic      TEXT     NOT NULL CHECK (char_length(topic) <= 200),
    conclusion TEXT     NOT NULL CHECK (char_length(conclusion) <= 20000),
    CHECK (topic <> '' OR conclusion <> ''),  -- совсем пустой блок не храним
    UNIQUE (week_start, position)
);

-- +goose Down
DROP TABLE week_review_blocks;
DROP TABLE week_reviews;
