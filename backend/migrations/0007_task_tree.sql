-- +goose Up

-- Подзадачи (SPEC §18.3, фаза B2): дело можно разбить на шаги, у шагов — свои шаги.
-- Выполненность и объём родителя не хранятся — считаются по листьям (tasks.Rollup);
-- done_at осмыслен только у листьев. CASCADE: удаление дела удаляет его шаги (с подтверждением на фронте).
-- Глубину (≤ 10) и отсутствие циклов проверяет Go: шаг всегда создаётся под существующим делом,
-- а parent_id потом не меняется — цикл через API не собрать.
ALTER TABLE tasks
    ADD COLUMN parent_id BIGINT REFERENCES tasks (id) ON DELETE CASCADE,
    ADD CONSTRAINT tasks_parent_not_self CHECK (parent_id <> id);

CREATE INDEX tasks_parent_idx ON tasks (parent_id);

-- +goose Down
ALTER TABLE tasks DROP COLUMN parent_id;
