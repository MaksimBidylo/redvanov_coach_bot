package main

import (
	"strings"
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestTelegramGroupArchive(t *testing.T) {
	pool := telegramTestDatabase(t)
	if _, err := pool.Exec(t.Context(), `INSERT INTO groups (name) VALUES ('Утро'), ('Вечер'), ('Контроль');
		INSERT INTO clients (name, birth_date, group_id, archived) VALUES ('Иван', '2010-03-15', 1, FALSE), ('Анна', '2000-01-01', 1, TRUE), ('Контроль', '1990-04-05', 3, FALSE)`); err != nil {
		t.Fatal(err)
	}
	process := telegramTestSession(t, pool, 8)
	message := &models.Message{ID: 10, From: &models.User{ID: 7}, Chat: models.Chat{ID: 7, Type: models.ChatTypePrivate}}
	var keyboard, oldConfirm string
	for _, tc := range []struct{ data, want, button, sql, flags, clients, absent string }{
		{"group:1", "Утро", "gar:1", "", "{f,f,f}", "{1,1,3}", ""},
		{"gar:1", "Куда перевести", "gar:1:2", "", "{f,f,f}", "{1,1,3}", ""},
		{"gar:1:2", "«Вечер»", "garc:1:2", "", "{f,f,f}", "{1,1,3}", ""},
		{"garc:1:2", "Не удалось", "group:1", "ALTER TABLE groups ADD CONSTRAINT group_archive_failure CHECK (id <> 1 OR NOT archived)", "{f,f,f}", "{1,1,3}", ""},
		{"group:1", "Утро", "gar:1", "ALTER TABLE groups DROP CONSTRAINT group_archive_failure", "{f,f,f}", "{1,1,3}", ""},
		{"gar:1", "Куда перевести", "gar:1:2", "", "{f,f,f}", "{1,1,3}", ""},
		{"gar:1:2", "«Вечер»", "garc:1:2", "", "{f,f,f}", "{1,1,3}", ""},
		{"garc:1:2", "", "", "", "{t,f,f}", "{2,2,3}", ""},
		{"garc:1:2", "Группа не найдена.", "groups", "", "{t,f,f}", "{2,2,3}", ""},
		{"old", "", "", "", "{t,f,f}", "{2,2,3}", ""},
		{"groups", "Вечер", "archived_groups", "", "{t,f,f}", "{2,2,3}", `"callback_data":"group:1`},
		{"client_view:1", "Иван", "move:1", "", "{t,f,f}", "{2,2,3}", ""},
		{"move:1", "Выберите группу", "client_view:1", "", "{t,f,f}", "{2,2,3}", `"callback_data":"move:1:1|`},
		{"archived_groups", "Утро", "group_restore:1", "", "{t,f,f}", "{2,2,3}", ""},
		{"group_restore:1", "Клиентов пока нет.", "gar:1", "", "{f,f,f}", "{2,2,3}", ""},
		{"group:2", "Вечер", "gar:2", "", "{f,f,f}", "{2,2,3}", ""},
		{"gar:2", "Куда перевести", "gar:2:0", "", "{f,f,f}", "{2,2,3}", ""},
		{"gar:2:0", "«Без группы»", "garc:2:0", "", "{f,f,f}", "{2,2,3}", ""},
		{"garc:2:0", "архив", "groups", "", "{f,t,f}", "{0,0,3}", ""},
	} {
		if tc.sql != "" {
			if _, err := pool.Exec(t.Context(), tc.sql); err != nil {
				t.Fatal(err)
			}
		}
		data := tc.data
		if data == "old" {
			data = oldConfirm
		} else if strings.HasPrefix(data, "gar") || strings.HasPrefix(data, "group_restore:") || strings.HasPrefix(data, "move:") {
			data = telegramTestCallback(t, keyboard, data)
		}
		got, next, _ := process(&models.Update{CallbackQuery: &models.CallbackQuery{
			ID: "callback", From: *message.From, Data: data, Message: models.MaybeInaccessibleMessage{Message: message},
		}})
		if (tc.want == "" && got != "") || (tc.want != "" && !strings.Contains(got+next, tc.want)) || (tc.absent != "" && strings.Contains(next, tc.absent)) {
			t.Fatalf("%s: got %q, want %q without %q", tc.data, got+next, tc.want, tc.absent)
		}
		if got != "" {
			keyboard = next
		}
		if tc.button != "" {
			telegramTestCallback(t, keyboard, tc.button)
		}
		if oldConfirm == "" && tc.data == "gar:1:2" {
			oldConfirm = telegramTestCallback(t, keyboard, "garc:1:2")
		}
		var flags, clients string
		err := pool.QueryRow(t.Context(), "SELECT (SELECT array_agg(archived ORDER BY id)::text FROM groups), (SELECT array_agg(COALESCE(group_id, 0) ORDER BY id)::text FROM clients)").Scan(&flags, &clients)
		if err != nil || flags != tc.flags || clients != tc.clients {
			t.Fatalf("%s: flags=%s, clients=%s, want %s/%s, error=%v", tc.data, flags, clients, tc.flags, tc.clients, err)
		}
	}
	var names, profiles string
	err := pool.QueryRow(t.Context(), "SELECT (SELECT string_agg(id::text || '/' || name, '|' ORDER BY id) FROM groups), (SELECT string_agg(name || '/' || birth_date::text || '/' || archived::text, '|' ORDER BY id) FROM clients)").Scan(&names, &profiles)
	if err != nil || names != "1/Утро|2/Вечер|3/Контроль" || profiles != "Иван/2010-03-15/false|Анна/2000-01-01/true|Контроль/1990-04-05/false" {
		t.Fatalf("details changed: groups=%q, clients=%q, error=%v", names, profiles, err)
	}
}
