package main

import (
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestTelegramPersonalTotal(t *testing.T) {
	pool := telegramTestDatabase(t)
	if _, err := pool.Exec(t.Context(), `INSERT INTO groups (name) VALUES ('Утро');
		INSERT INTO clients (name, group_id, archived) VALUES ('Архивный', 1, TRUE), ('Без группы', NULL, FALSE);
		INSERT INTO group_payments (client_id, group_id, amount_rub, received_on, training_month)
		VALUES (1, 1, 8888, '2027-01-15', '2027-01-01')`); err != nil {
		t.Fatal(err)
	}
	const seedPayments = `INSERT INTO personal_payments (client_id, amount_rub, received_on, save_key) VALUES
		(1, 2000, '2026-12-31', 'dec'), (1, 3000, '2027-01-01', 'jan-start'),
		(2, 4000, '2027-01-31', 'jan-end'), (2, 5000, '2027-02-01', 'feb')`
	process := telegramTestSession(t, pool)
	message := &models.Message{From: &models.User{ID: 7}, Chat: models.Chat{ID: 7, Type: models.ChatTypePrivate}}
	var keyboard string
	var screenID int
	const title = "Персональные оплаты\n"
	const choose = title + "Выберите период."
	const prompt = "Введите месяц получения денег (ММ.ГГГГ)."
	for _, tc := range []struct{ data, input, want, button, sql string }{
		{"", "/start", "Главное меню", "summary_personal", ""},
		{"summary_personal", "", choose, "summary_all", ""},
		{"summary_all", "", title + "Период: За всё время\nИтого: 0 ₽", "summary_back", ""},
		{"summary_back", "", choose, "summary_all", ""},
		{"summary_all", "", title + "Период: За всё время\nИтого: 14000 ₽", "summary_back", seedPayments},
		{"summary_back", "", choose, "summary_month", ""},
		{"summary_month", "", prompt, "summary_back", ""},
		{"summary_back", "", choose, "summary_month", ""},
		{"summary_month", "", prompt, "summary_back", ""},
		{"", "01.2027", title + "Период: 01.2027\nИтого: 7000 ₽", "summary_export", ""},
		{"summary_back", "", choose, "summary_month", ""},
		{"summary_month", "", prompt, "summary_back", ""},
		{"", "03.2027", title + "Период: 03.2027\nИтого: 0 ₽", "summary_back", ""},
		{"summary_back", "", choose, "menu", ""},
		{"menu", "", "Главное меню", "summary_personal", ""},
		{"", "01.2027", "", "", ""},
	} {
		if tc.sql != "" {
			if _, err := pool.Exec(t.Context(), tc.sql); err != nil {
				t.Fatal(err)
			}
		}
		message.ID, message.Text = 100, tc.input
		update := &models.Update{Message: message}
		if tc.data != "" {
			message.ID = screenID
			update.CallbackQuery = &models.CallbackQuery{ID: "callback", From: *message.From,
				Data: telegramTestCallback(t, keyboard, tc.data), Message: models.MaybeInaccessibleMessage{Message: message}}
		}
		got, next, replyID := process(update)
		if got != tc.want {
			t.Fatalf("%s/%q: got %q, want %q", tc.data, tc.input, got, tc.want)
		}
		if got != "" {
			keyboard, screenID = next, replyID
		}
		if tc.button != "" {
			telegramTestCallback(t, keyboard, tc.button)
		}
	}
}
