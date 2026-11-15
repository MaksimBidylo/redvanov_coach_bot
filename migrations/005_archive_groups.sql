-- +goose Up
ALTER TABLE groups ADD COLUMN archived BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE groups DROP COLUMN archived;
