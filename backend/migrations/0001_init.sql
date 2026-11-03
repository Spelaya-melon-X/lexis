-- +goose Up

-- Все «временные» колонки без DEFAULT now(): время всегда приходит из
-- platform/clock параметром запроса, чтобы тесты с Fixed-часами были детерминированы.

CREATE TABLE words (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    term             TEXT        NOT NULL CHECK (term <> ''),
    forms            TEXT[]      NOT NULL DEFAULT '{}' CHECK (cardinality(forms) IN (0, 3)),  -- 3 формы глагола, иначе пусто
    translation      TEXT        NOT NULL CHECK (translation <> ''),
    level            SMALLINT    NOT NULL DEFAULT 1 CHECK (level BETWEEN 1 AND 5),
    streak           SMALLINT    NOT NULL DEFAULT 0 CHECK (streak >= 0),
    next_review_at   TIMESTAMPTZ NOT NULL,                -- у нового слова = created_at
    last_reviewed_at TIMESTAMPTZ,
    last_good_at     TIMESTAMPTZ,                         -- для правил «разные дни» / «интервал ≥ N дней»
    created_at       TIMESTAMPTZ NOT NULL
);

-- Дубликат = тот же term + translation без учёта регистра (§4).
CREATE UNIQUE INDEX words_term_translation_uq ON words (lower(term), lower(translation));
-- «Карточки на сегодня»: архив (level 5) никогда не due, поэтому частичный индекс.
CREATE INDEX words_due_idx ON words (next_review_at) WHERE level < 5;

CREATE TABLE word_examples (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    word_id         BIGINT   NOT NULL REFERENCES words (id) ON DELETE CASCADE,
    position        SMALLINT NOT NULL,      -- порядок примеров у слова
    -- Предложение с разметкой выделения: "I [go] home" (§4). Спаны считает words.ParseExample при чтении.
    sentence        TEXT     NOT NULL CHECK (sentence <> ''),
    -- при слиянии дубликатов новые примеры добавляются, повторные — нет
    UNIQUE (word_id, sentence)
);

CREATE TABLE groups (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name       TEXT        NOT NULL UNIQUE CHECK (name <> ''),  -- уже нормализовано парсером
    color      TEXT,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE word_groups (
    word_id  BIGINT NOT NULL REFERENCES words (id) ON DELETE CASCADE,
    group_id BIGINT NOT NULL REFERENCES groups (id) ON DELETE CASCADE,
    PRIMARY KEY (word_id, group_id)
);
-- PK покрывает поиск по word_id; для «слова группы» нужен обратный индекс.
CREATE INDEX word_groups_group_idx ON word_groups (group_id);

CREATE TABLE training_sessions (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- SET NULL: удаление группы не стирает историю и статистику
    group_id     BIGINT      REFERENCES groups (id) ON DELETE SET NULL,
    strategy     TEXT        NOT NULL CHECK (strategy IN ('en_ru', 'ru_en', 'context')),
    free         BOOLEAN     NOT NULL DEFAULT false,
    started_at   TIMESTAMPTZ NOT NULL,
    finished_at  TIMESTAMPTZ,
    total        INT         NOT NULL DEFAULT 0,
    good         INT         NOT NULL DEFAULT 0,
    hard         INT         NOT NULL DEFAULT 0,
    again        INT         NOT NULL DEFAULT 0,
    duration_sec INT         NOT NULL DEFAULT 0
);
-- статус группы «повторена сегодня» и счётчики за неделю/месяц
CREATE INDEX training_sessions_group_finished_idx ON training_sessions (group_id, finished_at);

CREATE TABLE reviews (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    session_id       BIGINT      NOT NULL REFERENCES training_sessions (id) ON DELETE CASCADE,
    word_id          BIGINT      NOT NULL REFERENCES words (id) ON DELETE CASCADE,
    answer           TEXT        NOT NULL CHECK (answer IN ('good', 'hard', 'again')),
    -- первый ответ на слово в сессии: только он двигает уровень и идёт в «долю Знаю»
    first_in_session BOOLEAN     NOT NULL,
    level_before     SMALLINT    NOT NULL,
    level_after      SMALLINT    NOT NULL,
    answered_at      TIMESTAMPTZ NOT NULL
);
CREATE INDEX reviews_session_idx ON reviews (session_id);
CREATE INDEX reviews_word_idx ON reviews (word_id);

CREATE TABLE achievements (
    code        TEXT        PRIMARY KEY,
    unlocked_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE settings (
    key   TEXT  PRIMARY KEY,
    value JSONB NOT NULL
);

-- +goose Down
DROP TABLE settings;
DROP TABLE achievements;
DROP TABLE reviews;
DROP TABLE training_sessions;
DROP TABLE word_groups;
DROP TABLE groups;
DROP TABLE word_examples;
DROP TABLE words;
