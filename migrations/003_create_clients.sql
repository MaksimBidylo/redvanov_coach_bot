-- +goose Up
CREATE TABLE clients (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name TEXT NOT NULL,
    birth_date DATE,
    group_id BIGINT REFERENCES groups (id)
);

-- +goose Down
DROP TABLE clients;
