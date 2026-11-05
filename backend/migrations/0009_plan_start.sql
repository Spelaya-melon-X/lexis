-- +goose Up

-- Распорядок дня (SPEC §18.3, B3, доработка): дело ставится в любое время на шкале, а не подряд.
-- start_min — начало в минутах от полуночи, по получасам. Пересечения проверяет Go (tasks.PlanInput.Normalize):
-- пункты не перекрываются — при перетаскивании фронт меняет их местами.
ALTER TABLE plan_items ADD COLUMN start_min SMALLINT;

-- Старые планы шли подряд от начала дня (9:00 по умолчанию) в порядке position — так их и раскладываем.
UPDATE plan_items p SET start_min = LEAST(1410, 540 + 30 * (
    SELECT COALESCE(sum(q.blocks), 0) FROM plan_items q WHERE q.day = p.day AND q.position < p.position));

ALTER TABLE plan_items
    ALTER COLUMN start_min SET NOT NULL,
    ADD CONSTRAINT plan_items_start_check CHECK (start_min BETWEEN 0 AND 1410 AND start_min % 30 = 0);

-- +goose Down
ALTER TABLE plan_items DROP COLUMN start_min;
