-- +goose Up

-- Вложенные группы английского (SPEC §18.2): группа может лежать внутри другой.
-- NULL — корень. Циклы и глубину (≤ 3) проверяет Go (groups/tree) под advisory-блокировкой:
-- CHECK-ом их не выразить. RESTRICT, а не CASCADE: при удалении родителя сервис сам решает,
-- поднять подгруппы или удалить их (§18.2), и случайно снести поддерево каскадом нельзя.
ALTER TABLE groups
    ADD COLUMN parent_id BIGINT REFERENCES groups (id) ON DELETE RESTRICT,
    ADD CONSTRAINT groups_parent_not_self CHECK (parent_id <> id);

CREATE INDEX groups_parent_idx ON groups (parent_id);

-- +goose Down
ALTER TABLE groups DROP COLUMN parent_id;
