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

func groupPaymentsExcel(ctx context.Context, pool *pgxpool.Pool, groupID int64, name string, month *time.Time) ([]byte, error) {
	values, err := groupPaymentRows(ctx, pool, groupID, name, month)
	if err != nil {
		return nil, err
	}
	return paymentWorkbook(paymentSheet{"Sheet1", "E", values})
}

func groupPaymentRows(ctx context.Context, pool *pgxpool.Pool, groupID int64, name string, month *time.Time) ([][]any, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rows, err := pool.Query(ctx, `SELECT c.name, p.amount_rub, p.received_on, g.name, p.training_month
		FROM group_payments p JOIN clients c ON c.id = p.client_id JOIN groups g ON g.id = p.group_id
		WHERE ($1::bigint = 0 OR p.group_id = $1) AND ($2::date IS NULL OR p.training_month = $2::date)
		ORDER BY p.received_on, p.id`, groupID, month)
	if err != nil {
		return nil, err
	}
	payments, err := pgx.CollectRows(rows, pgx.RowToStructByPos[groupPaymentExportRow])
	if err != nil {
		return nil, err
	}
	title, period := "Групповые оплаты", "За всё время"
	if groupID > 0 {
		title = "Группа: " + name
	}
	if month != nil {
		period = month.Format("01.2006")
	}
	values := [][]any{
		{title},
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
	return values, nil
}

type paymentSheet struct {
	name, lastColumn string
	rows             [][]any
}

func paymentWorkbook(sheets ...paymentSheet) (data []byte, err error) {
	file := excelize.NewFile()
	defer func() { err = errors.Join(err, file.Close()) }()
	for i, sheet := range sheets {
		if i == 0 {
			err = file.SetSheetName("Sheet1", sheet.name)
		} else {
			_, err = file.NewSheet(sheet.name)
		}
		if err != nil {
			return nil, err
		}
		for j, row := range sheet.rows {
			if err := file.SetSheetRow(sheet.name, fmt.Sprintf("A%d", j+1), &row); err != nil {
				return nil, err
			}
		}
		if err := file.SetColWidth(sheet.name, "A", sheet.lastColumn, 24); err != nil {
			return nil, err
		}
	}
	buffer, err := file.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func excelAmount(value *big.Int) any {
	if len(value.String()) > 15 {
		return value.String()
	}
	return value.Int64()
}
