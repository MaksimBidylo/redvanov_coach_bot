package main

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type personalPayment struct {
	ClientID   int64
	AmountRub  int64
	ReceivedOn time.Time
}

func createPersonalPayment(ctx context.Context, pool *pgxpool.Pool, payment personalPayment, saveKey string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := pool.Exec(ctx, `INSERT INTO personal_payments (client_id, amount_rub, received_on, save_key)
		SELECT id, $2, $3, $4 FROM clients WHERE id = $1 ON CONFLICT (save_key) DO NOTHING`,
		payment.ClientID, payment.AmountRub, payment.ReceivedOn, saveKey)
	if err == nil && result.RowsAffected() == 0 {
		var saved bool
		err = pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM personal_payments WHERE save_key = $1)", saveKey).Scan(&saved)
		if err == nil && !saved {
			return pgx.ErrNoRows
		}
	}
	return err
}
