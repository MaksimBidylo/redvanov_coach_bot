package main

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type clientPayment struct {
	ID            int64
	Personal      bool
	AmountRub     int64
	ReceivedOn    time.Time
	GroupName     *string
	TrainingMonth *time.Time
}

func listClientPayments(ctx context.Context, pool *pgxpool.Pool, clientID, page int64) ([]clientPayment, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rows, err := pool.Query(ctx, `SELECT p.id, FALSE, p.amount_rub, p.received_on, g.name, p.training_month
		FROM group_payments p JOIN groups g ON g.id = p.group_id WHERE p.client_id = $1
		UNION ALL SELECT id, TRUE, amount_rub, received_on, NULL::text, NULL::date FROM personal_payments WHERE client_id = $1
		ORDER BY 4 DESC, 2, 1 DESC LIMIT 11 OFFSET $2`, clientID, page*10)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[clientPayment])
}

func deleteClientPayment(ctx context.Context, pool *pgxpool.Pool, clientID, paymentID int64, personal bool) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	table := "group_payments"
	if personal {
		table = "personal_payments"
	}
	_, err := pool.Exec(ctx, "DELETE FROM "+table+" WHERE id = $1 AND client_id = $2", paymentID, clientID)
	return err
}
