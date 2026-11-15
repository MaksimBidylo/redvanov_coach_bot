package main

import (
	"strings"
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestTelegramClientSearch(t *testing.T) {
	pool := telegramTestDatabase(t)
	if _, err := pool.Exec(t.Context(), `INSERT INTO groups (name) VALUES ('Утро');
		INSERT INTO clients (name, birth_date, group_id) VALUES
		('Иванов Иван', '2010-03-15', 1), ('иванов Пётр', NULL, NULL), ('100%_Имя', NULL, NULL);
		INSERT INTO clients (name) SELECT 'Лимит' || n FROM generate_series(1, 20) AS s(n)`); err != nil {
		t.Fatal(err)
	}
	process := telegramTestSession(t, pool)
	message := &models.Message{ID: 10, From: &models.User{ID: 7}, Chat: models.Chat{ID: 7, Type: models.ChatTypePrivate}}
	var resultKeyboard, cardKeyboard, oldResultKeyboard, oldCardKeyboard string
	for _, tc := range []struct {
		text, data, want, sql, fragment string
		old                             bool
		count                           int
	}{
		{"", "search", clientSearchPrompt, "", "", false, 0},
		{"", "menu", "", "", "", true, 0},
		{"", "client_skip", "", "", "", false, 0},
		{" \n ", "", clientSearchPrompt, "", "", false, 0},
		{"ИВАН", "", "Результаты поиска:", "", `"text":"Иванов Иван (15.03.2010) — Утро","callback_data":"search_view:1|`, false, 2},
		{"", "search_view:1", "Иванов Иван\nДата рождения: 15.03.2010\nГруппа: Утро", "", `"callback_data":"search_results|`, false, 0},
		{"", "search_results", "Результаты поиска:", "", `"callback_data":"search_view:2|`, false, 2},
		{"", "search_view:2", "иванов Пётр\nДата рождения: не указана\nГруппа: без группы", "", `"callback_data":"search_results|`, false, 0},
		{"", "search_results", "Результаты поиска:", "", "иванов Пётр — без группы", false, 2},
		{"", "menu", "Добро пожаловать!", "", `"callback_data":"search"`, false, 0},
		{"ИВАН", "", "", "", "", false, 0},
		{"", "search", clientSearchPrompt, "", "", false, 0},
		{"", "search_view:1", "", "", "", true, 0},
		{"%_", "", "Результаты поиска:", "", `"callback_data":"search_view:3|`, false, 1},
		{"", "search_results", "", "", "", true, 0},
		{"", "search_view:3", "100%_Имя\nДата рождения: не указана\nГруппа: без группы", "", `"callback_data":"search_results|`, false, 0},
		{"", "search_results", "Результаты поиска:", "", `"callback_data":"search_view:3|`, false, 1},
		{"", "search", clientSearchPrompt, "", "", false, 0},
		{"ЛИМИТ", "", "Результаты поиска:", "", "", false, 20},
		{"", "search", clientSearchPrompt, "", "", false, 0},
		{"ЛИМИТ", "", "Найдено больше 20 клиентов. Уточните запрос.\n" + clientSearchPrompt, "INSERT INTO clients (name) VALUES ('Лимит21')", "", false, 0},
		{"Нет такого", "", "Клиенты не найдены.\n" + clientSearchPrompt, "", "", false, 0},
		{"ИВАН", "", "Не удалось найти клиентов. Попробуйте ещё раз.\n" + clientSearchPrompt, "ALTER TABLE clients RENAME COLUMN name TO broken", "", false, 0},
		{"ИВАН", "", "Результаты поиска:", "ALTER TABLE clients RENAME COLUMN broken TO name", "иванов Пётр — без группы", false, 2},
		{"", "search", clientSearchPrompt, "", "", false, 0},
		{"/start", "", "Добро пожаловать!", "", "", false, 0},
		{"ИВАН", "", "", "", "", false, 0},
		{"", "search_results", "", "", "", true, 0},
	} {
		if tc.sql != "" {
			if _, err := pool.Exec(t.Context(), tc.sql); err != nil {
				t.Fatal(err)
			}
		}
		if tc.data == "search" {
			oldResultKeyboard, oldCardKeyboard = resultKeyboard, cardKeyboard
		}
		message.Text = tc.text
		message.ID = 100
		update := &models.Update{Message: message}
		if tc.data != "" {
			message.ID = 10
			data := tc.data
			if strings.HasPrefix(tc.data, "search_") {
				keyboards := map[string]string{"search_view": resultKeyboard, "search_results": cardKeyboard}
				if tc.old {
					keyboards = map[string]string{"search_view": oldResultKeyboard, "search_results": oldCardKeyboard}
				}
				action, _, _ := strings.Cut(tc.data, ":")
				data = telegramTestCallback(t, keyboards[action], tc.data)
			}
			update.CallbackQuery = &models.CallbackQuery{ID: "callback", From: *message.From, Data: data, Message: models.MaybeInaccessibleMessage{Message: message}}
			if tc.old && tc.data == "menu" {
				update.CallbackQuery.Message.Message = &models.Message{ID: message.ID - 1, Chat: message.Chat}
			}
		}
		got, keyboard, _ := process(update)
		if got == "Результаты поиска:" {
			resultKeyboard = keyboard
		}
		if strings.Contains(keyboard, `"callback_data":"search_results|`) {
			cardKeyboard = keyboard
		}
		if got != tc.want || !strings.Contains(keyboard, tc.fragment) {
			t.Fatalf("query %q, callback %q: got %q, want %q; keyboard=%s", tc.text, tc.data, got, tc.want, keyboard)
		}
		if strings.Contains(got, clientSearchPrompt) && keyboard != `{"inline_keyboard":[[{"text":"Назад","callback_data":"menu"}]]}` {
			t.Fatalf("search prompt must offer only Back: %s", keyboard)
		}
		if tc.count > 0 && strings.Count(keyboard, `"callback_data":"search_view:`) != tc.count {
			t.Fatalf("query %q: want %d results in %s", tc.text, tc.count, keyboard)
		}
	}
}
