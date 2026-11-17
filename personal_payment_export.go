package main

import (
	"context"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type personalPaymentExportRow struct {
	ClientName string
	AmountRub  int64
	ReceivedOn time.Time
}

func personalPaymentsExcel(ctx context.Context, pool *pgxpool.Pool, clientID int64, name string, month *time.Time) ([]byte, error) {
	values, err := personalPaymentRows(ctx, pool, clientID, name, month)
	if err != nil {
		return nil, err
	}
	return paymentWorkbook(paymentSheet{"Sheet1", "C", values})
}

func personalPaymentRows(ctx context.Context, pool *pgxpool.Pool, clientID int64, name string, month *time.Time) ([][]any, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rows, err := pool.Query(ctx, `SELECT c.name, p.amount_rub, p.received_on
		FROM personal_payments p JOIN clients c ON c.id = p.client_id
		WHERE ($1::bigint = 0 OR p.client_id = $1) AND ($2::date IS NULL OR
		(p.received_on >= $2::date AND p.received_on < $2::date + INTERVAL '1 month'))
		ORDER BY p.received_on, p.id`, clientID, month)
	if err != nil {
		return nil, err
	}
	payments, err := pgx.CollectRows(rows, pgx.RowToStructByPos[personalPaymentExportRow])
	if err != nil {
		return nil, err
	}
	title, period := "Персональные оплаты", "За всё время"
	if clientID > 0 {
		title += " клиента «" + name + "»"
	}
	if month != nil {
		period = month.Format("01.2006")
	}
	values := [][]any{
		{title},
		{"Период: " + period},
		{"Клиент", "Сумма (₽)", "Дата получения"},
	}
	var total big.Int
	for _, payment := range payments {
		amount := big.NewInt(payment.AmountRub)
		total.Add(&total, amount)
		values = append(values, []any{payment.ClientName, excelAmount(amount), payment.ReceivedOn.Format("02.01.2006")})
	}
	values = append(values, []any{"Итого", excelAmount(&total)})
	return values, nil
}
