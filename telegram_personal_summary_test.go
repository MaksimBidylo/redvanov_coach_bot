package main

import (
	"strings"
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestTelegramPersonalSummary(t *testing.T) {
	pool := telegramTestDatabase(t)
	if _, err := pool.Exec(t.Context(), `INSERT INTO groups (name) VALUES ('Утро');
		INSERT INTO clients (name, group_id, archived) VALUES ('Иван', 1, TRUE), ('Другой', NULL, FALSE), ('Пустой', NULL, FALSE);
		INSERT INTO personal_payments (client_id, amount_rub, received_on, save_key) VALUES
		(1, 2000, '2026-12-31', 'dec'), (1, 3000, '2027-01-01', 'jan-start'),
		(1, 4000, '2027-01-31', 'jan-end'), (1, 5000, '2027-02-01', 'feb'), (2, 9900, '2027-01-15', 'other');
		INSERT INTO group_payments (client_id, group_id, amount_rub, received_on, training_month) VALUES (1, 1, 8888, '2027-01-15', '2027-01-01')`); err != nil {
		t.Fatal(err)
	}
	process := telegramTestSession(t, pool, 6)
	message := &models.Message{From: &models.User{ID: 7}, Chat: models.Chat{ID: 7, Type: models.ChatTypePrivate}}
	var keyboard, old string
	const title = "Персональные оплаты клиента «Иван»\n"
	for _, tc := range []struct{ data, input, want, button, sql string }{
		{"search", "", clientSearchPrompt, "", ""},
		{"", "Иван", "Результаты поиска:", "search_view:1", ""},
		{"search_view:1", "", "Иван (арх)\n", "summary_personal:1", ""},
		{"summary_personal:1", "", title + "Выберите период.", "summary_month", ""},
		{"summary_month", "", "Введите месяц получения денег (ММ.ГГГГ).", "summary_back", ""},
		{"", "01.2027", "", "", ""},
		{"", "01.2027", title + "Период: 01.2027\nИтого: 7000 ₽", "summary_back", ""},
		{"old", "", "", "", ""},
		{"summary_back", "", title + "Выберите период.", "summary_month", ""},
		{"summary_month", "", "Введите месяц получения денег", "summary_back", ""},
		{"", "12.2026", title + "Период: 12.2026\nИтого: 2000 ₽", "summary_back", ""},
		{"summary_back", "", title + "Выберите период.", "summary_month", ""},
		{"summary_month", "", "Введите месяц получения денег", "summary_back", ""},
		{"", "02.2027", title + "Период: 02.2027\nИтого: 5000 ₽", "summary_back", ""},
		{"summary_back", "", title + "Выберите период.", "summary_all", ""},
		{"summary_all", "", "Не удалось загрузить сводку. Попробуйте ещё раз.", "summary_back", "ALTER TABLE personal_payments RENAME TO hidden_payments"},
		{"summary_back", "", title + "Выберите период.", "summary_all", "ALTER TABLE hidden_payments RENAME TO personal_payments"},
		{"summary_all", "", title + "Период: За всё время\nИтого: 14000 ₽", "summary_back", ""},
		{"summary_back", "", "Выберите период.", "summary_all", ""},
		{"summary_all", "", "Итого: 14500 ₽", "summary_back", "UPDATE personal_payments SET amount_rub = 3500 WHERE save_key = 'jan-start'"},
		{"summary_back", "", "Выберите период.", "summary_all", ""},
		{"summary_all", "", "Итого: 10500 ₽", "summary_back", "DELETE FROM personal_payments WHERE save_key = 'jan-end'"},
		{"summary_back", "", title + "Выберите период.", "search_view:1", ""},
		{"search_view:1", "", "Иван (арх)\n", "search_results", ""},
		{"search_results", "", "Результаты поиска:", "search_view:1", ""},
		{"search_view:1", "", "Иван (арх)\n", "ledger:1", ""},
		{"ledger:1", "", "Оплаты:", "ledger_view:0", ""},
		{"ledger_view:0", "", "Персональная оплата", "ledger_edit:0", ""},
		{"ledger_edit:0", "", "Выберите поле", "ledger_edit_amount", ""},
		{"ledger_edit_amount", "", "Введите сумму оплаты.", "ledger_edit_back", ""},
		{"client_view:1", "", "Иван (арх)\n", "summary_personal:1", ""},
		{"summary_personal:1", "", "Выберите период.", "summary_month", ""},
		{"summary_month", "", "Введите месяц получения денег", "summary_back", ""},
		{"", "01.2027", title + "Период: 01.2027\nИтого: 3500 ₽", "summary_back", ""},
		{"client_view:3", "", "Пустой\n", "summary_personal:3", ""},
		{"summary_personal:3", "", "Персональные оплаты клиента «Пустой»\nВыберите период.", "summary_all", ""},
		{"summary_all", "", "Персональные оплаты клиента «Пустой»\nПериод: За всё время\nИтого: 0 ₽", "summary_back", ""},
		{"summary_back", "", "Выберите период.", "summary_all", ""},
		{"summary_all", "", "Итого: 18446744073709551614 ₽", "summary_back", "INSERT INTO personal_payments (client_id, amount_rub, received_on, save_key) VALUES (3, 9223372036854775807, '2027-01-01', 'max1'), (3, 9223372036854775807, '2027-01-01', 'max2')"},
		{"summary_back", "", "Выберите период.", "client_view:3", ""},
		{"client_view:3", "", "Пустой\n", "ungrouped", ""},
	} {
		if tc.sql != "" {
			if _, err := pool.Exec(t.Context(), tc.sql); err != nil {
				t.Fatal(err)
			}
		}
		message.ID, message.Text = 100, tc.input
		update := &models.Update{Message: message}
		if tc.data != "" {
			message.ID = 10
			data := tc.data
			if data == "old" {
				data = old
			} else if strings.Contains(keyboard, `"callback_data":"`+data+`|`) {
				data = telegramTestCallback(t, keyboard, data)
			}
			update.CallbackQuery = &models.CallbackQuery{ID: "callback", From: *message.From, Data: data, Message: models.MaybeInaccessibleMessage{Message: message}}
		}
		got, next, _ := process(update)
		if (tc.want == "" && got != "") || (tc.want != "" && !strings.Contains(got, tc.want)) {
			t.Fatalf("%s/%q: got %q, want %q", tc.data, tc.input, got, tc.want)
		}
		if got != "" {
			keyboard = next
		}
		if tc.button != "" {
			telegramTestCallback(t, keyboard, tc.button)
		}
		if tc.data == "summary_personal:1" {
			old = telegramTestCallback(t, keyboard, "summary_month")
		}
	}
}
