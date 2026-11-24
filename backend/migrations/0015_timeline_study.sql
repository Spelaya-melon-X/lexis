-- +goose Up
CREATE TABLE timeline_years (year INTEGER PRIMARY KEY CHECK (year BETWEEN 1 AND 9999));
CREATE TABLE timeline_goals (
 id BIGINT PRIMARY KEY CHECK (id BETWEEN 1 AND 9007199254740991),
 year INTEGER NOT NULL CHECK (year BETWEEN 1 AND 9999),
 month INTEGER CHECK (month BETWEEN 1 AND 12),
 title TEXT NOT NULL CHECK (char_length(btrim(title)) BETWEEN 1 AND 200),
 note TEXT NOT NULL CHECK (char_length(note) <= 20000),
 dream_id BIGINT REFERENCES dreams ON DELETE SET NULL DEFERRABLE INITIALLY DEFERRED
);
CREATE INDEX timeline_goals_period ON timeline_goals (year, month);
CREATE SEQUENCE study_ids;
CREATE TABLE study_groups (
 id BIGINT PRIMARY KEY CHECK (id BETWEEN 1 AND 9007199254740991),
 title TEXT NOT NULL CHECK (char_length(btrim(title)) BETWEEN 1 AND 100),
 parent_id BIGINT REFERENCES study_groups DEFERRABLE INITIALLY DEFERRED,
 CHECK (parent_id <> id)
);
CREATE TABLE study_cards (
 id BIGINT PRIMARY KEY CHECK (id BETWEEN 1 AND 9007199254740991),
 group_id BIGINT NOT NULL REFERENCES study_groups DEFERRABLE INITIALLY DEFERRED,
 kind TEXT NOT NULL CHECK (kind IN ('definition','theorem','remark','property','lemma')),
 front TEXT NOT NULL CHECK (char_length(btrim(front)) BETWEEN 1 AND 20000),
 back TEXT NOT NULL CHECK (char_length(btrim(back)) BETWEEN 1 AND 20000)
);
CREATE INDEX study_cards_group ON study_cards (group_id);
-- +goose Down
DROP TABLE study_cards, study_groups, timeline_goals, timeline_years;
DROP SEQUENCE study_ids;
