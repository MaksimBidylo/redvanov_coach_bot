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

func createGroupPayment(ctx context.Context, pool *pgxpool.Pool, payment groupPayment) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := pool.Exec(ctx, `INSERT INTO group_payments (client_id, group_id, amount_rub, received_on, training_month)
		SELECT c.id, c.group_id, $3, $4, $5 FROM clients c JOIN groups g ON g.id = c.group_id
		WHERE c.id = $1 AND c.group_id = $2 AND NOT g.archived`,
		payment.ClientID, payment.GroupID, payment.AmountRub, payment.ReceivedOn, payment.TrainingMonth)
	if err == nil && result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return err
}
