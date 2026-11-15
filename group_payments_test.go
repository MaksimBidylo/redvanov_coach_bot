package main

import (
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestGroupPaymentStorage(t *testing.T) {
	pool := telegramTestDatabase(t)
	if _, err := pool.Exec(t.Context(), `INSERT INTO groups (name, archived) VALUES ('Утро', FALSE), ('Вечер', FALSE), ('Архив', TRUE);
		INSERT INTO clients (name, group_id) VALUES ('Иван', 1), ('Анна', 1), ('Без группы', NULL), ('Архивная группа', 3)`); err != nil {
		t.Fatal(err)
	}
	received := time.Date(2026, time.November, 15, 0, 0, 0, 0, time.UTC)
	month := time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC)
	var want []groupPayment
	for _, tc := range []struct {
		name                      string
		clientID, groupID, amount int64
		month                     time.Time
		valid                     bool
	}{
		{"first", 1, 1, 3000, month, true},
		{"same month", 1, 1, 1000, month, true},
		{"zero", 1, 1, 0, month, false},
		{"negative", 1, 1, -1, month, false},
		{"missing client", 99, 1, 3000, month, false},
		{"missing group", 1, 99, 3000, month, false},
		{"other group", 1, 2, 3000, month, false},
		{"ungrouped", 3, 0, 3000, month, false},
		{"archived group", 4, 3, 3000, month, false},
		{"not a month", 1, 1, 3000, received, false},
	} {
		payment := groupPayment{ClientID: tc.clientID, GroupID: tc.groupID, AmountRub: tc.amount, ReceivedOn: received, TrainingMonth: tc.month}
		if err := createGroupPayment(t.Context(), pool, payment); (err == nil) != tc.valid {
			t.Fatalf("%s: valid=%v, error=%v", tc.name, tc.valid, err)
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
	if err := closeGroup(t.Context(), pool, 1, 2, true); err != nil {
		t.Fatal(err)
	}
	rows, err := pool.Query(t.Context(), "SELECT client_id, group_id, amount_rub, received_on, training_month FROM group_payments ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	got, err := pgx.CollectRows(rows, pgx.RowToStructByPos[groupPayment])
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("payments after client transfer and archiving: got %+v, want %+v, error=%v", got, want, err)
	}
}
