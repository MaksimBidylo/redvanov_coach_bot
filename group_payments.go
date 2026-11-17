package main

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type groupPayment struct {
	ClientID      int64
	GroupID       int64
	AmountRub     int64
	ReceivedOn    time.Time
	TrainingMonth time.Time
}

func createGroupPayment(ctx context.Context, pool *pgxpool.Pool, payment groupPayment, saveKey string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := pool.Exec(ctx, `INSERT INTO group_payments (client_id, group_id, amount_rub, received_on, training_month, save_key)
		SELECT c.id, c.group_id, $3, $4, $5, $6 FROM clients c JOIN groups g ON g.id = c.group_id
		WHERE c.id = $1 AND c.group_id = $2 AND NOT g.archived ON CONFLICT (save_key) DO NOTHING`,
		payment.ClientID, payment.GroupID, payment.AmountRub, payment.ReceivedOn, payment.TrainingMonth, saveKey)
	if err == nil && result.RowsAffected() == 0 {
		var saved bool
		err = pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM group_payments WHERE save_key = $1)", saveKey).Scan(&saved)
		if err == nil && !saved {
			return pgx.ErrNoRows
		}
	}
	return err
}

func groupPaymentTotal(ctx context.Context, pool *pgxpool.Pool, groupID int64, month *time.Time) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var total string
	err := pool.QueryRow(ctx, `SELECT COALESCE(SUM(amount_rub), 0)::text FROM group_payments
		WHERE group_id = $1 AND ($2::date IS NULL OR training_month = $2::date)`, groupID, month).Scan(&total)
	return total, err
}
