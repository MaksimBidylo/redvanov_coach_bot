package main

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestPersonalPaymentStorage(t *testing.T) {
	pool := telegramTestDatabase(t)
	if _, err := pool.Exec(t.Context(), `INSERT INTO groups (name) VALUES ('Утро'), ('Вечер');
		INSERT INTO clients (name, group_id, archived) VALUES ('Иван', 1, FALSE), ('Без группы', NULL, FALSE), ('Архивный', NULL, TRUE)`); err != nil {
		t.Fatal(err)
	}
	received := time.Date(2026, time.November, 15, 0, 0, 0, 0, time.UTC)
	var want []personalPayment
	for _, tc := range []struct {
		key              string
		clientID, amount int64
		valid            bool
	}{
		{"grouped", 1, 3000, true},
		{"same data", 1, 3000, true},
		{"ungrouped", 2, 1000, true},
		{"archived", 3, 2000, true},
		{"zero", 1, 0, false},
		{"negative", 1, -1, false},
		{"missing client", 99, 3000, false},
		{"", 1, 3000, false},
	} {
		payment := personalPayment{ClientID: tc.clientID, AmountRub: tc.amount, ReceivedOn: received}
		err := createPersonalPayment(t.Context(), pool, payment, tc.key)
		if (err == nil) != tc.valid || (tc.clientID == 99 && !errors.Is(err, pgx.ErrNoRows)) {
			t.Fatalf("%q: valid=%v, error=%v", tc.key, tc.valid, err)
		}
		if tc.valid {
			want = append(want, payment)
		}
	}
	if err := setClientGroup(t.Context(), pool, 1, 2); err != nil {
		t.Fatal(err)
	}
	if err := setClientArchived(t.Context(), pool, 1, true); err != nil {
		t.Fatal(err)
	}
	if err := closeGroup(t.Context(), pool, 2, 0, true); err != nil {
		t.Fatal(err)
	}
	if err := createPersonalPayment(t.Context(), pool, want[0], "grouped"); err != nil {
		t.Fatalf("retry after transfer and archiving: %v", err)
	}
	rows, err := pool.Query(t.Context(), "SELECT client_id, amount_rub, received_on FROM personal_payments ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	got, err := pgx.CollectRows(rows, pgx.RowToStructByPos[personalPayment])
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("payments after client transfer and archiving: got %+v, want %+v, error=%v", got, want, err)
	}
}
