-- +goose Up
CREATE TABLE study_reviews (
 id TEXT PRIMARY KEY CHECK (id ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'),
 card_id BIGINT NOT NULL CHECK (card_id BETWEEN 1 AND 9007199254740991),
 reviewed_at TIMESTAMPTZ NOT NULL,
 known BOOLEAN NOT NULL,
 reverse BOOLEAN NOT NULL
);
-- History survives edits/deletion of cards; card_id is a historical identifier, not a foreign key.
CREATE INDEX study_reviews_at ON study_reviews (reviewed_at);
-- +goose Down
DROP TABLE study_reviews;
