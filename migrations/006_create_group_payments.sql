-- +goose Up
CREATE TABLE group_payments (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    client_id BIGINT NOT NULL REFERENCES clients (id),
    group_id BIGINT NOT NULL REFERENCES groups (id),
    amount_rub BIGINT NOT NULL CHECK (amount_rub > 0),
    received_on DATE NOT NULL,
    training_month DATE NOT NULL CHECK (EXTRACT(DAY FROM training_month) = 1)
);

-- +goose Down
DROP TABLE group_payments;
