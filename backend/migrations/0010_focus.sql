-- +goose Up

-- Ритуал фокуса и награды (SPEC §18.3, фаза B4). Время — параметром из clock.

-- Чек-лист настройки перед делом («поднял стол», «налил воды»…). Отметки в нём — на фронте (B9).
CREATE TABLE ritual_items (
    id       BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    text     TEXT   NOT NULL CHECK (text <> '' AND char_length(text) <= 100),
    position INT    NOT NULL
);

-- Фокус-сессия: настройка (started_at) → работа (work_started_at) → завершение (finished_at + outcome).
-- Таймеры фронт считает от этих отметок — перезагрузка вкладки ничего не ломает.
CREATE TABLE focus_sessions (
    id              BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- SET NULL + снимок названия: статистика переживает удаление дела.
    task_id         BIGINT      REFERENCES tasks (id) ON DELETE SET NULL,
    task_title      TEXT        NOT NULL CHECK (task_title <> ''),
    day             DATE        NOT NULL,
    planned_blocks  SMALLINT    NOT NULL CHECK (planned_blocks BETWEEN 1 AND 40),
    started_at      TIMESTAMPTZ NOT NULL,
    work_started_at TIMESTAMPTZ CHECK (work_started_at >= started_at),
    finished_at     TIMESTAMPTZ CHECK (finished_at >= started_at),
    outcome         TEXT        CHECK (outcome IN ('done', 'stopped', 'abandoned')),
    CHECK ((finished_at IS NULL) = (outcome IS NULL))
);
-- Одна активная сессия на всё приложение: уникальный индекс по константе среди незавершённых.
CREATE UNIQUE INDEX focus_sessions_active_uq ON focus_sessions ((true)) WHERE finished_at IS NULL;
CREATE INDEX focus_sessions_day_idx ON focus_sessions (day);
CREATE INDEX focus_sessions_task_idx ON focus_sessions (task_id);

CREATE TABLE focus_breaks (
    id         BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    session_id BIGINT      NOT NULL REFERENCES focus_sessions (id) ON DELETE CASCADE,
    started_at TIMESTAMPTZ NOT NULL,
    ended_at   TIMESTAMPTZ CHECK (ended_at >= started_at)
);
-- Не больше одного открытого перерыва в сессии.
CREATE UNIQUE INDEX focus_breaks_open_uq ON focus_breaks (session_id) WHERE ended_at IS NULL;

CREATE TABLE focus_distractions (
    id         BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    session_id BIGINT      NOT NULL REFERENCES focus_sessions (id) ON DELETE CASCADE,
    at         TIMESTAMPTZ NOT NULL,
    trigger    TEXT        NOT NULL CHECK (trigger <> '' AND char_length(trigger) <= 200),
    gave_in    BOOLEAN     NOT NULL  -- true — поддался, false — справился
);
CREATE INDEX focus_distractions_session_idx ON focus_distractions (session_id);

-- Магазин наград («дофаминовые приколюхи»).
CREATE TABLE rewards (
    id          BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title       TEXT        NOT NULL CHECK (title <> '' AND char_length(title) <= 100),
    price       INT         NOT NULL CHECK (price BETWEEN 1 AND 100000),
    position    INT         NOT NULL,
    archived_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL
);

-- Журнал очков: начисления за фокус и траты в магазине. Баланс = Σ delta (≥ 0 проверяет сервис при покупке).
CREATE TABLE points_ledger (
    id               BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    at               TIMESTAMPTZ NOT NULL,
    delta            INT         NOT NULL CHECK (delta <> 0),
    kind             TEXT        NOT NULL CHECK (kind IN ('focus', 'purchase')),
    title            TEXT        NOT NULL,                       -- снимок: дело или награда могут быть удалены
    focus_session_id BIGINT      UNIQUE REFERENCES focus_sessions (id) ON DELETE SET NULL,  -- начисление — одно на сессию
    reward_id        BIGINT      REFERENCES rewards (id) ON DELETE SET NULL
);

-- +goose Down
DROP TABLE points_ledger;
DROP TABLE rewards;
DROP TABLE focus_distractions;
DROP TABLE focus_breaks;
DROP TABLE focus_sessions;
DROP TABLE ritual_items;
