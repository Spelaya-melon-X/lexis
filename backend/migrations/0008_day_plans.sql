-- +goose Up

-- Распорядок дня (SPEC §18.3, фаза B3): какие дела взять в день и в каком порядке.
-- Без привязки ко времени: примерное время на шкале фронт считает, накапливая блоки от начала дня.
-- План — список, сохраняется целиком (PUT заменяет), поэтому position — просто порядок.
CREATE TABLE plan_items (
    day      DATE     NOT NULL,
    task_id  BIGINT   NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    position SMALLINT NOT NULL,
    blocks   SMALLINT NOT NULL CHECK (blocks BETWEEN 1 AND 40),  -- сколько блоков дела на этот день
    PRIMARY KEY (day, task_id)                                   -- одно дело — один пункт в дне
);
-- «Не доделал»: пункты прошлых дней по делу; и удаление планов родителя при разбиении на шаги.
CREATE INDEX plan_items_task_idx ON plan_items (task_id);

-- +goose Down
DROP TABLE plan_items;
