package main

import (
	"strings"
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestTelegramGroupSummary(t *testing.T) {
	pool := telegramTestDatabase(t)
	if _, err := pool.Exec(t.Context(), `INSERT INTO groups (name) VALUES ('Утро'), ('Вечер'), ('Пустая');
		INSERT INTO clients (name, group_id, archived) VALUES ('Переведённый', 2, FALSE), ('Архивный', NULL, TRUE), ('Другой', 2, FALSE);
		INSERT INTO group_payments (client_id, group_id, amount_rub, received_on, training_month) VALUES
		(1, 1, 3000, '2026-11-15', '2027-01-01'), (2, 1, 2000, '2027-02-15', '2027-01-01'),
		(1, 1, 1000, '2027-01-10', '2027-02-01'), (3, 2, 9900, '2027-01-10', '2027-01-01');
		INSERT INTO personal_payments (client_id, amount_rub, received_on, save_key) VALUES (1, 8888, '2027-01-10', 'personal')`); err != nil {
		t.Fatal(err)
	}
	process := telegramTestSession(t, pool, 4)
	message := &models.Message{From: &models.User{ID: 7}, Chat: models.Chat{ID: 7, Type: models.ChatTypePrivate}}
	var keyboard, old string
	const title = "Сводка по группе «Утро»\n"
	for _, tc := range []struct{ data, input, want, button, sql string }{
		{"group:1", "", "Утро", "summary:1", ""},
		{"summary:1", "", title + "Выберите период.", "summary_month", ""},
		{"summary_month", "", "Введите месяц занятий (ММ.ГГГГ).", "summary_back", ""},
		{"", "01.2027", "", "", ""},
		{"", "13.2027", "Некорректная дата", "summary_back", ""},
		{"", "01.0000", "Некорректная дата", "summary_back", ""},
		{"", "01.2027", title + "Период: 01.2027\nИтого: 5000 ₽", "summary_back", ""},
		{"old", "", "", "", ""},
		{"", "02.2027", "", "", ""},
		{"summary_back", "", title + "Выберите период.", "summary_month", ""},
		{"summary_month", "", "Введите месяц занятий", "summary_back", ""},
		{"", "02.2027", title + "Период: 02.2027\nИтого: 1000 ₽", "summary_back", ""},
		{"summary_back", "", title + "Выберите период.", "summary_all", ""},
		{"summary_all", "", "Не удалось загрузить сводку. Попробуйте ещё раз.", "summary_back", "ALTER TABLE group_payments RENAME TO hidden_payments"},
		{"summary_back", "", title + "Выберите период.", "summary_all", "ALTER TABLE hidden_payments RENAME TO group_payments"},
		{"summary_all", "", title + "Период: За всё время\nИтого: 6000 ₽", "summary_back", ""},
		{"summary_back", "", title + "Выберите период.", "summary_all", ""},
		{"summary_all", "", "Итого: 6500 ₽", "summary_back", "UPDATE group_payments SET amount_rub = 3500 WHERE id = 1"},
		{"summary_back", "", title + "Выберите период.", "summary_all", ""},
		{"summary_all", "", "Итого: 4500 ₽", "summary_back", "DELETE FROM group_payments WHERE id = 2"},
		{"summary_back", "", title + "Выберите период.", "group:1", ""},
		{"group:1", "", "Утро", "summary:1", ""},
		{"", "01.2027", "", "", ""},
		{"group:3", "", "Пустая", "summary:3", ""},
		{"summary:3", "", "Сводка по группе «Пустая»\nВыберите период.", "summary_all", ""},
		{"summary_all", "", "Сводка по группе «Пустая»\nПериод: За всё время\nИтого: 0 ₽", "summary_back", ""},
		{"summary_back", "", "Выберите период.", "summary_all", ""},
		{"summary_all", "", "Итого: 18446744073709551614 ₽", "summary_back", "INSERT INTO group_payments (client_id, group_id, amount_rub, received_on, training_month) VALUES (1, 3, 9223372036854775807, '2027-01-10', '2027-01-01'), (2, 3, 9223372036854775807, '2027-01-10', '2027-01-01')"},
		{"summary_back", "", "Выберите период.", "summary_month", ""},
		{"summary_month", "", "Введите месяц занятий", "summary_back", ""},
		{"groups", "", "Группы:", "group_create", ""},
		{"group_create", "", "Введите название группы.", "groups", ""},
		{"", "Новая", "Группа создана.", "groups", ""},
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
			} else if strings.HasPrefix(data, "summary") || data == "group:1" && keyboard != "" {
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
		if tc.data == "summary:1" {
			old = telegramTestCallback(t, keyboard, "summary_month")
		}
	}
}
