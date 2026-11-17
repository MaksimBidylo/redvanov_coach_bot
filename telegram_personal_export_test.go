package main

import (
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/xuri/excelize/v2"
)

func TestTelegramPersonalExport(t *testing.T) {
	pool := telegramTestDatabase(t)
	if _, err := pool.Exec(t.Context(), `INSERT INTO groups (name) VALUES ('Утро');
		INSERT INTO clients (name, group_id, archived) VALUES ('Анна', 1, TRUE), ('Борис', NULL, FALSE);
		INSERT INTO personal_payments (client_id, amount_rub, received_on, save_key) VALUES
		(1, 2000, '2026-12-31', 'dec'), (1, 3000, '2027-01-01', 'jan-start'),
		(1, 4000, '2027-01-31', 'jan-end'), (1, 5000, '2027-02-01', 'feb'), (2, 1000, '2027-01-15', 'other');
		INSERT INTO group_payments (client_id, group_id, amount_rub, received_on, training_month)
		VALUES (1, 1, 8888, '2027-01-15', '2027-01-01')`); err != nil {
		t.Fatal(err)
	}
	var rows [][]string
	var uploads int
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/bot123:test-token/sendDocument" {
			t.Fatalf("export must preserve the active screen: %s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		part, header, err := r.FormFile("document")
		if err != nil {
			t.Fatal(err)
		}
		book, err := excelize.OpenReader(part)
		if closeErr := part.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
		if err != nil || header.Filename != "personal-payments.xlsx" || r.FormValue("chat_id") != "7" {
			t.Fatalf("invalid personal export: %v, %v", header, err)
		}
		rows, err = book.GetRows("Sheet1")
		if closeErr := book.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
		if err != nil {
			t.Fatal(err)
		}
		uploads++
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"ok":true,"result":{"message_id":99}}`))}, nil
	})}
	b, err := bot.New("123:test-token", bot.WithSkipGetMe(), bot.WithHTTPClient(time.Second, client))
	if err != nil {
		t.Fatal(err)
	}
	jan, march := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC)
	for i, tc := range []struct {
		id                       int64
		month                    *time.Time
		name, period, total, sql string
		count                    int
	}{
		{1, &jan, "Анна", "01.2027", "7000", "", 6},
		{0, &jan, "", "01.2027", "8000", "", 7},
		{0, nil, "", "За всё время", "15000", "", 9},
		{0, &march, "", "03.2027", "0", "", 4},
		{0, &jan, "", "01.2027", "4500", "UPDATE personal_payments SET amount_rub = 3500 WHERE save_key = 'jan-start'; DELETE FROM personal_payments WHERE save_key = 'jan-end'", 6},
	} {
		if tc.sql != "" {
			if _, err := pool.Exec(t.Context(), tc.sql); err != nil {
				t.Fatal(err)
			}
		}
		summary := &paymentSummary{id: tc.id, name: tc.name, personal: true, month: tc.month, exportReady: true}
		h := &telegramHandler{ownerID: 7, pool: pool, messageID: 10, revision: 42, summary: summary}
		h.handleSummary(t.Context(), b, 7, 0, "summary_export", "")
		title := "Персональные оплаты"
		if tc.id != 0 {
			title += " клиента «Анна»"
		}
		if uploads != i+1 || h.messageID != 10 || h.revision != 42 || h.summary != summary || len(rows) != tc.count || rows[0][0] != title || rows[1][0] != "Период: "+tc.period || !reflect.DeepEqual(rows[2], []string{"Клиент", "Сумма (₽)", "Дата получения"}) || !reflect.DeepEqual(rows[len(rows)-1], []string{"Итого", tc.total}) {
			t.Fatalf("export %d: rows=%v, screen=%d/%d, uploads=%d", i, rows, h.messageID, h.revision, uploads)
		}
		if i == 0 && !reflect.DeepEqual(rows[3:5], [][]string{{"Анна", "3000", "01.01.2027"}, {"Анна", "4000", "31.01.2027"}}) {
			t.Fatalf("wrong personal payment rows: %v", rows)
		}
	}
}
