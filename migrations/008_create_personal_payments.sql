-- +goose Up
CREATE TABLE personal_payments (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    client_id BIGINT NOT NULL REFERENCES clients (id),
    amount_rub BIGINT NOT NULL CHECK (amount_rub > 0),
    received_on DATE NOT NULL,
    save_key TEXT NOT NULL UNIQUE CHECK (save_key <> '')
);

-- +goose Down
DROP TABLE personal_payments;
