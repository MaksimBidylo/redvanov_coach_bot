-- +goose Up
ALTER TABLE group_payments ADD COLUMN save_key TEXT UNIQUE CHECK (save_key <> '');

-- +goose Down
ALTER TABLE group_payments DROP COLUMN save_key;
