-- +goose Up
ALTER TABLE clients ADD COLUMN archived BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE clients DROP COLUMN archived;
