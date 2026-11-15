package main

import (
	"strings"
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestTelegramClientPayments(t *testing.T) {
	pool := telegramTestDatabase(t)
	if _, err := pool.Exec(t.Context(), `INSERT INTO groups (name) VALUES ('Утро'), ('Вечер');
		INSERT INTO clients (name, group_id) VALUES ('Иван', 1), ('Другой', NULL);
		INSERT INTO group_payments (client_id, group_id, amount_rub, received_on, training_month) VALUES (1, 1, 2001, '2026-11-15', '2027-01-01');
		INSERT INTO personal_payments (client_id, amount_rub, received_on, save_key)
		SELECT 1, 4000+n, CASE WHEN n = 11 THEN '2026-11-15'::date ELSE '2026-11-14'::date END, 'p'||n FROM generate_series(1, 11) s(n);
		INSERT INTO personal_payments (client_id, amount_rub, received_on, save_key) VALUES (2, 9999, '2026-11-16', 'other');
		UPDATE groups SET archived = TRUE WHERE id = 1; UPDATE clients SET group_id = 2 WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	process := telegramTestSession(t, pool, 5)
	message := &models.Message{From: &models.User{ID: 7}, Chat: models.Chat{ID: 7, Type: models.ChatTypePrivate}}
	var keyboard, old string
	for _, tc := range []struct {
		data, input, want string
		rows              int
	}{
		{"search", "", clientSearchPrompt, 0},
		{"", "Иван", "Результаты поиска:", 0},
		{"search_view:1", "", "Иван\n", 0},
		{"ledger:1", "", "Оплаты:", 10},
		{"old", "", "", 0},
		{"ledger_page:1", "", "", 0},
		{"ledger_view:0", "", "Групповая оплата\nСумма: 2001 ₽\nДата получения денег: 15.11.2026\nГруппа: Утро\nМесяц занятий: 01.2027", 0},
		{"ledger_page:0", "", "Оплаты:", 10},
		{"ledger_page:1", "", "Оплаты:", 2},
		{"ledger_view:0", "", "Персональная оплата\nСумма: 4002 ₽\nДата получения денег: 14.11.2026", 0},
		{"ledger_page:1", "", "Оплаты:", 2},
		{"ledger_client", "", "Иван\n", 0},
		{"search_results", "", "Результаты поиска:", 0},
	} {
		message.ID, message.Text = 100, tc.input
		update := &models.Update{Message: message}
		if tc.data != "" {
			message.ID = 10
			data := tc.data
			if data == "old" {
				data = old
			} else if strings.HasPrefix(data, "ledger") || strings.HasPrefix(data, "search_") {
				data = telegramTestCallback(t, keyboard, data)
			}
			update.CallbackQuery = &models.CallbackQuery{ID: "callback", From: *message.From, Data: data, Message: models.MaybeInaccessibleMessage{Message: message}}
		}
		got, next, _ := process(update)
		if (tc.want == "" && got != "") || (tc.want != "" && !strings.Contains(got, tc.want)) || (strings.HasPrefix(tc.data, "ledger_view:") && got != tc.want) || strings.Contains(got+next, "9999") || (tc.rows > 0 && strings.Count(next, `"callback_data":"ledger_view:`) != tc.rows) {
			t.Fatalf("%s: got %q, keyboard=%s", tc.data, got, next)
		}
		if got != "" {
			keyboard = next
		}
		if tc.data == "search_view:1" {
			old = telegramTestCallback(t, keyboard, "ledger:1")
		}
	}
}
