package main

import (
	"strings"
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestTelegramArchivedGroupSummary(t *testing.T) {
	pool := telegramTestDatabase(t)
	if _, err := pool.Exec(t.Context(), `INSERT INTO groups (name) VALUES ('Утро'), ('Вечер');
		INSERT INTO clients (name, group_id, archived) VALUES ('Иван', 1, FALSE), ('Анна', 1, TRUE);
		INSERT INTO group_payments (client_id, group_id, amount_rub, received_on, training_month) VALUES
		(1, 1, 3000, '2026-11-15', '2027-01-01'), (2, 1, 2000, '2027-02-15', '2027-01-01'),
		(1, 1, 1000, '2027-01-10', '2027-02-01'), (1, 2, 9900, '2027-01-10', '2027-01-01');
		INSERT INTO personal_payments (client_id, amount_rub, received_on, save_key) VALUES (1, 8888, '2027-01-10', 'personal')`); err != nil {
		t.Fatal(err)
	}
	if err := closeGroup(t.Context(), pool, 1, 2, true); err != nil {
		t.Fatal(err)
	}
	process := telegramTestSession(t, pool)
	message := &models.Message{From: &models.User{ID: 7}, Chat: models.Chat{ID: 7, Type: models.ChatTypePrivate}}
	var keyboard string
	const title = "Сводка по группе «Утро»\n"
	for _, tc := range []struct{ data, input, want, button string }{
		{"archived_groups", "", "Архив групп:", "group:1"},
		{"group:1", "", "Утро (арх)", "summary:1"},
		{"summary:1", "", title + "Выберите период.", "summary_month"},
		{"summary_month", "", groupSummaryMonthPrompt, "summary_back"},
		{"", "01.2027", title + "Период: 01.2027\nИтого: 5000 ₽", "summary_back"},
		{"summary_back", "", title + "Выберите период.", "summary_all"},
		{"summary_all", "", title + "Период: За всё время\nИтого: 6000 ₽", "summary_back"},
		{"summary_back", "", title + "Выберите период.", "summary_month"},
		{"summary_month", "", groupSummaryMonthPrompt, "summary_back"},
		{"", "03.2027", title + "Период: 03.2027\nИтого: 0 ₽", "summary_back"},
		{"summary_back", "", title + "Выберите период.", "group:1"},
		{"group:1", "", "Утро (арх)", "archived_groups"},
		{"archived_groups", "", "Архив групп:", "group:1"},
		{"group:1", "", "Утро (арх)", "group_restore:1"},
		{"group_restore:1", "", "Утро\n\nКлиентов пока нет.", "summary:1"},
	} {
		message.ID, message.Text = 100, tc.input
		update := &models.Update{Message: message}
		if tc.data != "" {
			message.ID = 10
			data := tc.data
			if keyboard != "" {
				data = telegramTestCallback(t, keyboard, data)
			}
			update.CallbackQuery = &models.CallbackQuery{ID: "callback", From: *message.From, Data: data, Message: models.MaybeInaccessibleMessage{Message: message}}
		}
		got, next, _ := process(update)
		if got != tc.want {
			t.Fatalf("%s/%q: got %q, want %q", tc.data, tc.input, got, tc.want)
		}
		keyboard = next
		telegramTestCallback(t, keyboard, tc.button)
		if tc.data == "group:1" && strings.Count(keyboard, `"callback_data":`) != 3 {
			t.Fatalf("archived card must have only summary, restore and Back: %s", keyboard)
		}
		var archived bool
		var clients string
		err := pool.QueryRow(t.Context(), "SELECT archived, (SELECT array_agg(group_id ORDER BY id)::text FROM clients) FROM groups WHERE id = 1").Scan(&archived, &clients)
		if err != nil || archived != (tc.data != "group_restore:1") || clients != "{2,2}" {
			t.Fatalf("%s: archive=%v, clients=%s, error=%v", tc.data, archived, clients, err)
		}
	}
}
