-- +goose Up
ALTER TABLE groups DROP CONSTRAINT groups_name_key;
CREATE UNIQUE INDEX groups_name_lower_key ON groups (lower(name));

-- +goose Down
DROP INDEX groups_name_lower_key;
ALTER TABLE groups ADD CONSTRAINT groups_name_key UNIQUE (name);
