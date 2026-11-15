package main

import (
	"strings"
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestTelegramGroupDelete(t *testing.T) {
	pool := telegramTestDatabase(t)
	if _, err := pool.Exec(t.Context(), `INSERT INTO groups (name) VALUES ('Утро'), ('Вечер'), ('Пустая'), ('Другая');
		INSERT INTO clients (name, birth_date, group_id) VALUES ('Иван', '2010-03-15', 1), ('Анна', '2000-01-01', 1), ('Василий', '1999-02-02', 2), ('Контроль', '1990-04-05', 4)`); err != nil {
		t.Fatal(err)
	}
	process := telegramTestSession(t, pool, 5)
	message := &models.Message{ID: 10, From: &models.User{ID: 7}, Chat: models.Chat{ID: 7, Type: models.ChatTypePrivate}}
	var keyboard, oldConfirm string
	for _, tc := range []struct{ data, want, button, sql, groups, clients string }{
		{"group:3", "Пустая", "del:3", "", "{1,2,3,4}", "{1,1,2,4}"},
		{"del:3", "Удалить группу", "delc:3:0", "", "{1,2,3,4}", "{1,1,2,4}"},
		{"group:3", "Пустая", "del:3", "", "{1,2,3,4}", "{1,1,2,4}"},
		{"old", "", "", "", "{1,2,3,4}", "{1,1,2,4}"},
		{"del:3", "Удалить группу", "delc:3:0", "", "{1,2,3,4}", "{1,1,2,4}"},
		{"delc:3:0", "", "", "", "{1,2,4}", "{1,1,2,4}"},
		{"delc:3:0", "Группа не найдена.", "groups", "", "{1,2,4}", "{1,1,2,4}"},
		{"group:1", "Утро", "del:1", "", "{1,2,4}", "{1,1,2,4}"},
		{"del:1", "Куда перевести", "del:1:2", "", "{1,2,4}", "{1,1,2,4}"},
		{"del:1:2", "«Вечер»", "delc:1:2", "", "{1,2,4}", "{1,1,2,4}"},
		{"delc:1:2", "Не удалось", "group:1", "CREATE TABLE group_refs (group_id BIGINT REFERENCES groups); INSERT INTO group_refs VALUES (1)", "{1,2,4}", "{1,1,2,4}"},
		{"group:1", "Утро", "del:1", "DROP TABLE group_refs", "{1,2,4}", "{1,1,2,4}"},
		{"del:1", "Куда перевести", "del:1:2", "", "{1,2,4}", "{1,1,2,4}"},
		{"del:1:2", "«Вечер»", "delc:1:2", "", "{1,2,4}", "{1,1,2,4}"},
		{"delc:1:2", "Группа удалена.", "groups", "", "{2,4}", "{2,2,2,4}"},
		{"group:2", "Вечер", "del:2", "", "{2,4}", "{2,2,2,4}"},
		{"del:2", "Куда перевести", "del:2:0", "", "{2,4}", "{2,2,2,4}"},
		{"del:2:0", "«Без группы»", "delc:2:0", "", "{2,4}", "{2,2,2,4}"},
		{"delc:2:0", "Группа удалена.", "groups", "", "{4}", "{0,0,0,4}"},
	} {
		if tc.sql != "" {
			if _, err := pool.Exec(t.Context(), tc.sql); err != nil {
				t.Fatal(err)
			}
		}
		data := tc.data
		if data == "old" {
			data = oldConfirm
		} else if strings.HasPrefix(data, "del") {
			data = telegramTestCallback(t, keyboard, data)
		}
		got, next, _ := process(&models.Update{CallbackQuery: &models.CallbackQuery{
			ID: "callback", From: *message.From, Data: data, Message: models.MaybeInaccessibleMessage{Message: message},
		}})
		if (tc.want == "" && got != "") || (tc.want != "" && !strings.Contains(got, tc.want)) {
			t.Fatalf("%s: got %q, want %q", tc.data, got, tc.want)
		}
		if got != "" {
			keyboard = next
		}
		if tc.button != "" {
			telegramTestCallback(t, keyboard, tc.button)
		}
		if oldConfirm == "" && tc.data == "del:3" {
			oldConfirm = telegramTestCallback(t, keyboard, "delc:3:0")
		}
		if strings.Contains(next, `"callback_data":"del:1:1|`) {
			t.Fatal("the deleted group must not be a transfer target")
		}
		var groups, clients string
		err := pool.QueryRow(t.Context(), "SELECT (SELECT array_agg(id ORDER BY id)::text FROM groups), (SELECT array_agg(COALESCE(group_id, 0) ORDER BY id)::text FROM clients)").Scan(&groups, &clients)
		if err != nil || groups != tc.groups || clients != tc.clients {
			t.Fatalf("%s: groups=%s, clients=%s, want %s/%s, error=%v", tc.data, groups, clients, tc.groups, tc.clients, err)
		}
	}
	var profiles string
	err := pool.QueryRow(t.Context(), "SELECT string_agg(name || '/' || birth_date::text, '|' ORDER BY id) FROM clients").Scan(&profiles)
	if err != nil || profiles != "Иван/2010-03-15|Анна/2000-01-01|Василий/1999-02-02|Контроль/1990-04-05" {
		t.Fatalf("client details changed: %q, error=%v", profiles, err)
	}
}
