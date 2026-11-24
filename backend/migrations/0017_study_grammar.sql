-- +goose Up
ALTER TABLE study_groups ADD COLUMN grammar BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE study_cards DROP CONSTRAINT study_cards_kind_check;
ALTER TABLE study_cards ADD COLUMN grammar_content JSONB;
ALTER TABLE study_cards ADD CONSTRAINT study_cards_kind_check CHECK (
    (kind IN ('definition', 'theorem', 'remark', 'property', 'lemma') AND grammar_content IS NULL)
    OR (kind = 'grammar_rule' AND grammar_content IS NOT NULL AND jsonb_typeof(grammar_content) = 'object')
);

-- +goose Down
-- These checks abort the transaction if a downgrade would discard grammar data.
ALTER TABLE study_groups ADD CONSTRAINT study_groups_no_grammar CHECK (NOT grammar);
ALTER TABLE study_cards DROP CONSTRAINT study_cards_kind_check;
ALTER TABLE study_cards ADD CONSTRAINT study_cards_kind_check CHECK (kind IN ('definition', 'theorem', 'remark', 'property', 'lemma'));
ALTER TABLE study_cards DROP COLUMN grammar_content;
ALTER TABLE study_groups DROP COLUMN grammar;
