package main

import (
	"strings"
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestTelegramClientEdit(t *testing.T) {
	pool := telegramTestDatabase(t)
	if _, err := pool.Exec(t.Context(), `INSERT INTO groups (name) VALUES ('Утро');
		INSERT INTO clients (name, birth_date, group_id) VALUES ('Иван', '2010-03-15', 1), ('Анна', NULL, NULL), ('Другой', '2000-01-01', NULL)`); err != nil {
		t.Fatal(err)
	}
	process := telegramTestSession(t, pool, 4)
	message := &models.Message{From: &models.User{ID: 7}, Chat: models.Chat{ID: 7, Type: models.ChatTypePrivate}}
	var keyboard, oldEdit string
	for _, tc := range []struct{ data, input, want, button, sql, name, date string }{
		{"client_view:1", "", "Иван\n", "edit_name:1", "", "Иван", "2010-03-15"},
		{"edit_name:1", "", clientRenamePrompt, "client_view:1", "", "Иван", "2010-03-15"},
		{"", "  ", "Имя не должно быть пустым.", "client_view:1", "", "Иван", "2010-03-15"},
		{"", "  Анна  ", "", "", "", "Анна", "2010-03-15"},
		{"", "Анна", "Анна\n", "group:1", "", "Анна", "2010-03-15"},
		{"search", "", clientSearchPrompt, "menu", "", "Анна", "2010-03-15"},
		{"", "Анна", "Результаты поиска:", "search_view:1", "", "Анна", "2010-03-15"},
		{"search_view:1", "", "Анна\n", "search_results", "", "Анна", "2010-03-15"},
		{"edit_birth:1", "", "Изменение даты рождения.", "search_view:1", "", "Анна", "2010-03-15"},
		{"edit_date:1", "", clientEditDatePrompt, "edit_birth:1", "", "Анна", "2010-03-15"},
		{"edit_birth:1", "", "Изменение даты рождения.", "edit_date:1", "", "Анна", "2010-03-15"},
		{"old", "", "", "", "", "Анна", "2010-03-15"},
		{"", "01.01.2000", "", "", "", "Анна", "2010-03-15"},
		{"edit_date:1", "", clientEditDatePrompt, "edit_birth:1", "", "Анна", "2010-03-15"},
		{"", "31.02.2010", "Некорректная дата.", "edit_birth:1", "", "Анна", "2010-03-15"},
		{"", "29.02.2012", "Анна\n", "search_results", "", "Анна", "2012-02-29"},
		{"search_results", "", "Анна (29.02.2012) — Утро", "search_view:1", "", "Анна", "2012-02-29"},
		{"search_view:1", "", "Анна\n", "search_results", "", "Анна", "2012-02-29"},
		{"edit_birth:1", "", "Изменение даты рождения.", "edit_clear:1", "", "Анна", "2012-02-29"},
		{"edit_clear:1", "", "Не удалось", "edit_clear:1", "ALTER TABLE clients ADD CONSTRAINT edit_failure CHECK (birth_date IS NOT NULL) NOT VALID", "Анна", "2012-02-29"},
		{"edit_clear:1", "", "Анна\n", "search_results", "ALTER TABLE clients DROP CONSTRAINT edit_failure", "Анна", ""},
		{"search_results", "", "Анна — Утро", "search_view:1", "", "Анна", ""},
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
			if strings.HasPrefix(data, "edit_") || strings.HasPrefix(data, "search_") {
				data = telegramTestCallback(t, keyboard, data)
			} else if data == "old" {
				data = oldEdit
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
		if oldEdit == "" {
			oldEdit = telegramTestCallback(t, keyboard, "edit_name:1")
		}
		var savedName, savedDate string
		var groupID int64
		err := pool.QueryRow(t.Context(), "SELECT name, COALESCE(birth_date::text, ''), group_id FROM clients WHERE id = 1").Scan(&savedName, &savedDate, &groupID)
		if err != nil || savedName != tc.name || savedDate != tc.date || groupID != 1 {
			t.Fatalf("%s/%q: saved %q/%s/group%d, want %q/%s/group1, error=%v", tc.data, tc.input, savedName, savedDate, groupID, tc.name, tc.date, err)
		}
	}
	other, err := getClient(t.Context(), pool, 3)
	if err != nil || other.Name != "Другой" || other.BirthDate == nil || other.BirthDate.Format("02.01.2006") != "01.01.2000" || other.GroupID != nil {
		t.Fatalf("other client changed: %+v, error=%v", other, err)
	}
}
