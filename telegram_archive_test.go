package main

import (
	"strings"
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestTelegramClientArchive(t *testing.T) {
	pool := telegramTestDatabase(t)
	if _, err := pool.Exec(t.Context(), `INSERT INTO groups (name) VALUES ('Утро'), ('Вечер');
		INSERT INTO clients (name, birth_date, group_id) VALUES ('Иван', '2010-03-15', 1), ('Персональный', '2000-01-01', NULL), ('Контроль', '1990-04-05', 2)`); err != nil {
		t.Fatal(err)
	}
	process := telegramTestSession(t, pool, 3)
	message := &models.Message{From: &models.User{ID: 7}, Chat: models.Chat{ID: 7, Type: models.ChatTypePrivate}}
	var keyboard, oldArchive string
	for _, tc := range []struct{ data, input, want, button, sql, flags string }{
		{"group:1", "", "Клиенты:", "client_view:1", "", "{f,f,f}"},
		{"client_view:1", "", "Иван\n", "archive:1", "", "{f,f,f}"},
		{"archive:1", "", "", "", "", "{t,f,f}"},
		{"archive:1", "", "Иван (арх)\n", "restore:1", "", "{t,f,f}"},
		{"group:1", "", "Клиентов пока нет.", "del:1", "", "{t,f,f}"},
		{"del:1", "", "Куда перевести", "del:1:2", "", "{t,f,f}"},
		{"group:1", "", "Клиентов пока нет.", "del:1", "", "{t,f,f}"},
		{"search", "", clientSearchPrompt, "menu", "", "{t,f,f}"},
		{"", "Иван", "Иван (арх) (15.03.2010) — Утро", "search_view:1", "", "{t,f,f}"},
		{"search_view:1", "", "Иван (арх)\n", "restore:1", "", "{t,f,f}"},
		{"restore:1", "", "Иван\n", "search_results", "", "{f,f,f}"},
		{"old", "", "", "", "", "{f,f,f}"},
		{"search_results", "", "Иван (15.03.2010) — Утро", "search_view:1", "", "{f,f,f}"},
		{"group:1", "", "Клиенты:", "client_view:1", "", "{f,f,f}"},
		{"ungrouped", "", "Клиенты:", "client_view:2", "", "{f,f,f}"},
		{"client_view:2", "", "Персональный\n", "archive:2", "", "{f,f,f}"},
		{"archive:2", "", "Не удалось", "archive:2", "ALTER TABLE clients ADD CONSTRAINT archive_failure CHECK (id <> 2 OR NOT archived)", "{f,f,f}"},
		{"archive:2", "", "Персональный (арх)\n", "restore:2", "ALTER TABLE clients DROP CONSTRAINT archive_failure", "{f,t,f}"},
		{"ungrouped", "", "Клиентов пока нет.", "client:0", "", "{f,t,f}"},
		{"search", "", clientSearchPrompt, "menu", "", "{f,t,f}"},
		{"", "Персональный", "Персональный (арх) (01.01.2000) — без группы", "search_view:2", "", "{f,t,f}"},
		{"search_view:2", "", "Персональный (арх)\n", "restore:2", "", "{f,t,f}"},
		{"restore:2", "", "Персональный\n", "search_results", "", "{f,f,f}"},
		{"ungrouped", "", "Клиенты:", "client_view:2", "", "{f,f,f}"},
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
				data = oldArchive
			} else if strings.HasPrefix(data, "archive:") || strings.HasPrefix(data, "restore:") || strings.HasPrefix(data, "search_") || strings.HasPrefix(data, "del") {
				data = telegramTestCallback(t, keyboard, data)
			}
			update.CallbackQuery = &models.CallbackQuery{ID: "callback", From: *message.From, Data: data, Message: models.MaybeInaccessibleMessage{Message: message}}
		}
		got, next, _ := process(update)
		if (tc.want == "" && got != "") || (tc.want != "" && !strings.Contains(got+next, tc.want)) {
			t.Fatalf("%s/%q: got %q, want %q", tc.data, tc.input, got+next, tc.want)
		}
		if got != "" {
			keyboard = next
		}
		if tc.button != "" {
			telegramTestCallback(t, keyboard, tc.button)
		}
		if oldArchive == "" && tc.data == "client_view:1" {
			oldArchive = telegramTestCallback(t, keyboard, "archive:1")
		}
		var flags, groups, profiles string
		err := pool.QueryRow(t.Context(), "SELECT array_agg(archived ORDER BY id)::text, array_agg(COALESCE(group_id, 0) ORDER BY id)::text, string_agg(name || '/' || birth_date::text, '|' ORDER BY id) FROM clients").Scan(&flags, &groups, &profiles)
		if err != nil || flags != tc.flags || groups != "{1,0,2}" || profiles != "Иван/2010-03-15|Персональный/2000-01-01|Контроль/1990-04-05" {
			t.Fatalf("%s: flags=%s, groups=%s, profiles=%q, want flags=%s, error=%v", tc.data, flags, groups, profiles, tc.flags, err)
		}
	}
}
