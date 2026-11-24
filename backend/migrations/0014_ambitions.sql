-- +goose Up
CREATE SEQUENCE ambition_ids;
CREATE TABLE dream_categories (
 id BIGINT PRIMARY KEY, title TEXT NOT NULL CHECK (char_length(btrim(title)) BETWEEN 1 AND 60), position INTEGER NOT NULL
);
CREATE TABLE big_projects (
 id BIGINT PRIMARY KEY, title TEXT NOT NULL CHECK (char_length(btrim(title)) BETWEEN 1 AND 200),
 note TEXT NOT NULL CHECK (char_length(note) <= 20000), deadline DATE, position INTEGER NOT NULL
);
CREATE TABLE dreams (
 id BIGINT PRIMARY KEY, title TEXT NOT NULL CHECK (char_length(btrim(title)) BETWEEN 1 AND 200),
 note TEXT NOT NULL CHECK (char_length(note) <= 20000), deadline DATE,
 status TEXT NOT NULL CHECK (status IN ('want','active','fulfilled')), position INTEGER NOT NULL,
 category_id BIGINT REFERENCES dream_categories ON DELETE SET NULL DEFERRABLE INITIALLY DEFERRED,
 project_id BIGINT REFERENCES big_projects ON DELETE SET NULL DEFERRABLE INITIALLY DEFERRED
);
CREATE TABLE project_steps (
 id BIGINT PRIMARY KEY, project_id BIGINT NOT NULL REFERENCES big_projects ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
 parent_id BIGINT, title TEXT NOT NULL CHECK (char_length(btrim(title)) BETWEEN 1 AND 200),
 note TEXT NOT NULL CHECK (char_length(note) <= 20000), deadline DATE, position INTEGER NOT NULL, done BOOLEAN NOT NULL,
 UNIQUE (project_id, id), CHECK (parent_id <> id),
 FOREIGN KEY (project_id, parent_id) REFERENCES project_steps (project_id, id) DEFERRABLE INITIALLY DEFERRED
);
CREATE TABLE step_dependencies (
 project_id BIGINT NOT NULL, step_id BIGINT NOT NULL, requires_id BIGINT NOT NULL,
 PRIMARY KEY (step_id, requires_id), CHECK (step_id <> requires_id),
 FOREIGN KEY (project_id, step_id) REFERENCES project_steps (project_id, id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
 FOREIGN KEY (project_id, requires_id) REFERENCES project_steps (project_id, id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED
);
CREATE INDEX project_steps_parent ON project_steps (parent_id);
CREATE INDEX dreams_category ON dreams (category_id);
CREATE INDEX dreams_project ON dreams (project_id);
CREATE INDEX step_dependencies_requires ON step_dependencies (requires_id);
-- +goose Down
DROP TABLE step_dependencies, project_steps, dreams, big_projects, dream_categories;
DROP SEQUENCE ambition_ids;
