package main

import (
	"strings"
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestTelegramClientGroupChange(t *testing.T) {
	pool := telegramTestDatabase(t)
	if _, err := pool.Exec(t.Context(), `INSERT INTO groups (name) VALUES ('Утро'), ('Вечер'), ('Детская');
		INSERT INTO clients (name, birth_date, group_id) VALUES ('Иван', '2010-03-15', 1), ('Другой', NULL, 1)`); err != nil {
		t.Fatal(err)
	}
	process := telegramTestSession(t, pool)
	message := &models.Message{ID: 10, From: &models.User{ID: 7}, Chat: models.Chat{ID: 7, Type: models.ChatTypePrivate}}
	var keyboard, oldKeyboard string
	for _, tc := range []struct {
		data, text, want, back, sql string
		group                       int64
		old                         bool
	}{
		{"client_view:1", "", "Группа: Утро", "group:1", "", 1, false},
		{"move:1", "", "Выберите группу", "client_view:1", "", 1, false},
		{"move:1:-1", "", "", "", "", 1, false},
		{"move:1:2", "", "Группа: Вечер", "group:2", "", 2, false},
		{"move:1:1", "", "", "", "", 2, true},
		{"move:1", "", "Выберите группу", "client_view:1", "", 2, false},
		{"move:1:0", "", "Группа: без группы", "ungrouped", "", 0, false},
		{"move:1", "", "Не удалось загрузить группы", "client_view:1", "ALTER TABLE groups RENAME COLUMN name TO broken", 0, false},
		{"client_view:1", "", "Группа: без группы", "ungrouped", "ALTER TABLE groups RENAME COLUMN broken TO name", 0, false},
		{"search", "", clientSearchPrompt, "menu", "", 0, false},
		{"", "Иван", "Результаты поиска:", "menu", "", 0, false},
		{"search_view:1", "", "Группа: без группы", "search_results", "", 0, false},
		{"move:1", "", "Выберите группу", "search_view:1", "", 0, false},
		{"move:1:2", "", "Группа: Вечер", "search_results", "", 2, false},
		{"search_results", "", "Результаты поиска:", "menu", "", 2, false},
		{"search_view:1", "", "Группа: Вечер", "search_results", "", 2, false},
		{"move:1", "", "Выберите группу", "search_view:1", "", 2, false},
		{"move:1:3", "", "Не удалось изменить группу", "search_results", "DELETE FROM groups WHERE id = 3", 2, false},
		{"move:1", "", "Выберите группу", "search_view:1", "", 2, false},
		{"move:1:1", "", "Не удалось изменить группу. Попробуйте ещё раз.\n\nКлиент не найден", "search_results", "DELETE FROM clients WHERE id = 1", -1, false},
		{"search_results", "", "Клиенты не найдены", "menu", "", -1, false},
	} {
		if tc.sql != "" {
			if _, err := pool.Exec(t.Context(), tc.sql); err != nil {
				t.Fatal(err)
			}
		}
		message.ID, message.Text = 100, tc.text
		update := &models.Update{Message: message}
		if tc.data != "" {
			message.ID = 10
			data := tc.data
			if strings.HasPrefix(data, "move:") || strings.HasPrefix(data, "search_") {
				source := keyboard
				if tc.old {
					source = oldKeyboard
				}
				button := data
				if data == "move:1:-1" {
					button = "move:1:1"
				}
				data = strings.Replace(telegramTestCallback(t, source, button), button, data, 1)
			}
			update.CallbackQuery = &models.CallbackQuery{ID: "callback", From: *message.From, Data: data, Message: models.MaybeInaccessibleMessage{Message: message}}
		}
		got, next, _ := process(update)
		if (tc.want == "" && got != "") || (tc.want != "" && !strings.Contains(got, tc.want)) {
			t.Fatalf("%s: got %q, want %q", tc.data, got, tc.want)
		}
		if got != "" {
			keyboard = next
		}
		if tc.back != "" {
			telegramTestCallback(t, keyboard, tc.back)
		}
		if tc.data == "search_results" && tc.group == 2 && !strings.Contains(keyboard, "Иван (15.03.2010) — Вечер") {
			t.Fatalf("search shows the old group: %s", keyboard)
		}
		if strings.Contains(got, "Группа:") {
			telegramTestCallback(t, keyboard, "move:1")
		}
		if oldKeyboard == "" && tc.data == "move:1" {
			oldKeyboard = keyboard
		}
		if tc.group >= 0 {
			var c client
			err := pool.QueryRow(t.Context(), "SELECT name, birth_date, group_id FROM clients WHERE id = 1").Scan(&c.Name, &c.BirthDate, &c.GroupID)
			groupID := int64(0)
			if c.GroupID != nil {
				groupID = *c.GroupID
			}
			if err != nil || groupID != tc.group || c.Name != "Иван" || c.BirthDate == nil || c.BirthDate.Format("02.01.2006") != "15.03.2010" {
				t.Fatalf("%s: client=%+v, want group=%d, error=%v", tc.data, c, tc.group, err)
			}
		}
		var otherGroup int64
		err := pool.QueryRow(t.Context(), "SELECT group_id FROM clients WHERE id = 2").Scan(&otherGroup)
		if err != nil || otherGroup != 1 {
			t.Fatalf("other client changed: group=%d, error=%v", otherGroup, err)
		}
	}
}
