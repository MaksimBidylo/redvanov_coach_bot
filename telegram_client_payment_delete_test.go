package main

import (
	"strings"
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestTelegramClientPaymentDelete(t *testing.T) {
	pool := telegramTestDatabase(t)
	if _, err := pool.Exec(t.Context(), `INSERT INTO groups (name) VALUES ('Утро');
		INSERT INTO clients (name, group_id) VALUES ('Иван', 1), ('Другой', NULL);
		INSERT INTO group_payments (client_id, group_id, amount_rub, received_on, training_month)
		SELECT 1, 1, 2000+n, '2026-11-15', '2027-01-01' FROM generate_series(1, 11) s(n);
		INSERT INTO personal_payments (client_id, amount_rub, received_on, save_key)
		VALUES (1, 4000, '2026-11-14', 'own'), (2, 9999, '2026-11-16', 'other')`); err != nil {
		t.Fatal(err)
	}
	process := telegramTestSession(t, pool, 7)
	message := &models.Message{ID: 10, From: &models.User{ID: 7}, Chat: models.Chat{ID: 7, Type: models.ChatTypePrivate}}
	const group = "Групповая оплата\nСумма: 2011 ₽\nДата получения денег: 15.11.2026\nГруппа: Утро\nМесяц занятий: 01.2027"
	const personal = "Персональная оплата\nСумма: 4000 ₽\nДата получения денег: 14.11.2026"
	var keyboard, oldConfirm string
	for _, tc := range []struct {
		data, want, button     string
		groups, personal, rows int
		sql                    string
	}{
		{"client_view:1", "Иван\n", "ledger:1", 11, 2, 0, ""},
		{"ledger:1", "Оплаты:", "ledger_view:0", 11, 2, 10, ""},
		{"ledger_view:0", group, "ledger_delete:0", 11, 2, 0, ""},
		{"ledger_delete:0", group, "ledger_confirm:0", 11, 2, 0, ""},
		{"ledger_view:0", group, "ledger_delete:0", 11, 2, 0, ""},
		{"old", "", "", 11, 2, 0, ""},
		{"ledger_delete:0", group, "ledger_confirm:0", 11, 2, 0, ""},
		{"ledger_confirm:0", "", "", 10, 2, 0, ""},
		{"ledger_confirm:0", "Оплата удалена.", "ledger_page:1", 10, 2, 10, ""},
		{"old", "", "", 10, 2, 0, ""},
		{"ledger_page:1", "Оплаты:", "ledger_view:0", 10, 2, 1, ""},
		{"ledger_view:0", personal, "ledger_delete:0", 10, 2, 0, ""},
		{"ledger_delete:0", personal, "ledger_confirm:0", 10, 2, 0, ""},
		{"ledger_confirm:0", "Не удалось удалить оплату. Попробуйте ещё раз.\n\nУдалить оплату?\n\n" + personal, "ledger_confirm:0", 10, 2, 0, "CREATE TABLE delete_blocker (payment_id BIGINT REFERENCES personal_payments); INSERT INTO delete_blocker VALUES (1)"},
		{"ledger_confirm:0", "Оплата удалена.", "ledger_client", 10, 1, 10, "DROP TABLE delete_blocker"},
		{"ledger_client", "Иван\n", "group:1", 10, 1, 0, ""},
	} {
		if tc.sql != "" {
			if _, err := pool.Exec(t.Context(), tc.sql); err != nil {
				t.Fatal(err)
			}
		}
		data := tc.data
		if data == "old" {
			data = oldConfirm
		} else if strings.HasPrefix(data, "ledger") {
			data = telegramTestCallback(t, keyboard, data)
		}
		got, next, _ := process(&models.Update{CallbackQuery: &models.CallbackQuery{
			ID: "callback", From: *message.From, Data: data, Message: models.MaybeInaccessibleMessage{Message: message},
		}})
		if (tc.want == "" && got != "") || (tc.want != "" && !strings.Contains(got, tc.want)) || (tc.data == "ledger_view:0" && got != tc.want) || strings.Contains(got+next, "9999") {
			t.Fatalf("%s: got %q, keyboard=%s", tc.data, got, next)
		}
		if got != "" {
			keyboard = next
		}
		if tc.button != "" {
			telegramTestCallback(t, keyboard, tc.button)
		}
		if tc.data == "ledger_delete:0" {
			if !strings.Contains(got, "Удалить оплату?") {
				t.Fatalf("missing confirmation prompt: %q", got)
			}
			telegramTestCallback(t, keyboard, "ledger_view:0")
			oldConfirm = telegramTestCallback(t, keyboard, "ledger_confirm:0")
		}
		if tc.rows > 0 && strings.Count(next, `"callback_data":"ledger_view:`) != tc.rows {
			t.Fatalf("%s: want %d payments, keyboard=%s", tc.data, tc.rows, next)
		}
		if tc.personal == 1 && strings.Contains(next, `"callback_data":"ledger_page:1|`) {
			t.Fatalf("empty last page must return to the previous page: %s", next)
		}
		var groups, personals int
		var selectedGroup, selectedPersonal bool
		err := pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM group_payments), (SELECT count(*) FROM personal_payments),
			EXISTS (SELECT 1 FROM group_payments WHERE id = 11), EXISTS (SELECT 1 FROM personal_payments WHERE id = 1)`).Scan(&groups, &personals, &selectedGroup, &selectedPersonal)
		if err != nil || groups != tc.groups || personals != tc.personal || selectedGroup != (tc.groups == 11) || selectedPersonal != (tc.personal == 2) {
			t.Fatalf("%s: groups=%d/%t, personal=%d/%t, error=%v", tc.data, groups, selectedGroup, personals, selectedPersonal, err)
		}
	}
}
