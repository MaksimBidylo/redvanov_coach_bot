package main

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xuri/excelize/v2"
)

type groupPaymentExportRow struct {
	ClientName    string
	AmountRub     int64
	ReceivedOn    time.Time
	GroupName     string
	TrainingMonth time.Time
}

func groupPaymentsExcel(ctx context.Context, pool *pgxpool.Pool, groupID int64, name string, month *time.Time) (data []byte, err error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rows, err := pool.Query(ctx, `SELECT c.name, p.amount_rub, p.received_on, g.name, p.training_month
		FROM group_payments p JOIN clients c ON c.id = p.client_id JOIN groups g ON g.id = p.group_id
		WHERE p.group_id = $1 AND ($2::date IS NULL OR p.training_month = $2::date)
		ORDER BY p.received_on, p.id`, groupID, month)
	if err != nil {
		return nil, err
	}
	payments, err := pgx.CollectRows(rows, pgx.RowToStructByPos[groupPaymentExportRow])
	if err != nil {
		return nil, err
	}
	file := excelize.NewFile()
	defer func() { err = errors.Join(err, file.Close()) }()
	period := "За всё время"
	if month != nil {
		period = month.Format("01.2006")
	}
	values := [][]any{
		{"Группа: " + name},
		{"Период: " + period},
		{"Клиент", "Сумма (₽)", "Дата получения", "Группа", "Месяц занятий"},
	}
	var total big.Int
	for _, payment := range payments {
		amount := big.NewInt(payment.AmountRub)
		total.Add(&total, amount)
		values = append(values, []any{payment.ClientName, excelAmount(amount), payment.ReceivedOn.Format("02.01.2006"), payment.GroupName, payment.TrainingMonth.Format("01.2006")})
	}
	values = append(values, []any{"Итого", excelAmount(&total)})
	for i, row := range values {
		if err := file.SetSheetRow("Sheet1", fmt.Sprintf("A%d", i+1), &row); err != nil {
			return nil, err
		}
	}
	if err := file.SetColWidth("Sheet1", "A", "E", 24); err != nil {
		return nil, err
	}
	buffer, err := file.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func excelAmount(value *big.Int) any {
	// Excel rounds numeric cells beyond 15 decimal digits.
	if len(value.String()) > 15 {
		return value.String()
	}
	return value.Int64()
}
