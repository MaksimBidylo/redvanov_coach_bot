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

func personalPaymentTotal(ctx context.Context, pool *pgxpool.Pool, clientID int64, month *time.Time) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var total string
	err := pool.QueryRow(ctx, `SELECT COALESCE(SUM(amount_rub), 0)::text FROM personal_payments
		WHERE ($1::bigint = 0 OR client_id = $1) AND ($2::date IS NULL OR
		(received_on >= $2::date AND received_on < $2::date + INTERVAL '1 month'))`, clientID, month).Scan(&total)
	return total, err
}
