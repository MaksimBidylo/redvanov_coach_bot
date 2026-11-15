package main

import (
	"strings"
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestTelegramClientCreation(t *testing.T) {
	pool := telegramTestDatabase(t)
	if _, err := pool.Exec(t.Context(), "INSERT INTO groups (name) VALUES ('Взрослая утро')"); err != nil {
		t.Fatal(err)
	}
	process := telegramTestSession(t, pool)
	message := &models.Message{ID: 10, From: &models.User{ID: 7}, Chat: models.Chat{ID: 7, Type: models.ChatTypePrivate}}
	var promptID, oldPromptID int
	for _, tc := range []struct {
		text, data, want, sql, button string
		old                           bool
	}{
		{"", "group:1", "Взрослая утро\n\nКлиентов пока нет.", "", "client:1", false},
		{"", "client:1", clientNamePrompt, "", "", false},
		{"", "client_skip", "", "", "", false},
		{" \n ", "", "Имя не должно быть пустым.\n" + clientNamePrompt, "", "", false},
		{"  Иванов Иван  ", "", clientBirthDatePrompt, "", "client_skip", false},
		{"31.02.2010", "", "Некорректная дата.\n" + clientBirthDatePrompt, "", "client_skip", false},
		{"15.03.2010", "", "Не удалось сохранить клиента.\n" + clientBirthDatePrompt, "ALTER TABLE clients RENAME COLUMN name TO broken", "client_skip", false},
		{"", "client_skip", "Клиент создан.", "ALTER TABLE clients RENAME COLUMN broken TO name", "", false},
		{"", "client_skip", "", "", "", false},
		{"", "client:1", clientNamePrompt, "", "", false},
		{"Иванов Иван", "", clientBirthDatePrompt, "", "client_skip", false},
		{"", "client_skip", "", "", "", true},
		{"15.03.2010", "", "Клиент создан.", "", "", false},
		{"", "client:1", clientNamePrompt, "", "", false},
		{"Лишний", "", clientBirthDatePrompt, "", "client_skip", false},
		{"/start", "", "Добро пожаловать!", "", "", false},
		{"Лишний", "", "", "", "", false},
		{"", "client_skip", "", "", "", false},
	} {
		if tc.sql != "" {
			if _, err := pool.Exec(t.Context(), tc.sql); err != nil {
				t.Fatal(err)
			}
		}
		message.Text = tc.text
		update := &models.Update{Message: message}
		if tc.data != "" {
			update.CallbackQuery = &models.CallbackQuery{
				ID: "callback", From: *message.From, Data: tc.data,
				Message: models.MaybeInaccessibleMessage{Message: message},
			}
			if tc.data == "client_skip" {
				message.ID = promptID
				if tc.old {
					message.ID = oldPromptID
				}
			}
		}
		got, keyboard, replyID := process(update)
		if strings.Contains(keyboard, `"callback_data":"client_skip"`) {
			promptID = replyID
		}
		if got == "Клиент создан." {
			oldPromptID = promptID
		}
		if got != tc.want {
			t.Fatalf("text %q, callback %q: got %q, want %q", tc.text, tc.data, got, tc.want)
		}
		if tc.button != "" && !strings.Contains(keyboard, `"callback_data":"`+tc.button+`"`) {
			t.Fatalf("missing %q button in %s", tc.button, keyboard)
		}
	}
	var total, dated, undated int
	err := pool.QueryRow(t.Context(), "SELECT count(*), count(*) FILTER (WHERE name = 'Иванов Иван' AND group_id = 1 AND birth_date = DATE '2010-03-15'), count(*) FILTER (WHERE name = 'Иванов Иван' AND group_id = 1 AND birth_date IS NULL) FROM clients").Scan(&total, &dated, &undated)
	if err != nil || total != 2 || dated != 1 || undated != 1 {
		t.Fatalf("clients: total=%d, dated=%d, undated=%d, error=%v", total, dated, undated, err)
	}
}

func TestTelegramClientCard(t *testing.T) {
	const seed = `INSERT INTO groups (name) VALUES ('Утро'), ('Вечер');
		INSERT INTO clients (name, birth_date, group_id) VALUES
		('Иванов Иван', '2010-03-15', 2), ('Иванов Иван', NULL, 2), ('Персональный', NULL, NULL)`
	for _, tc := range []struct {
		name, data, want, back, sql string
	}{
		{"dated", "client_view:1", "Иванов Иван\nДата рождения: 15.03.2010\nГруппа: Вечер", "group:2", ""},
		{"undated", "client_view:2", "Иванов Иван\nДата рождения: не указана\nГруппа: Вечер", "group:2", ""},
		{"without group", "client_view:3", "Персональный\nДата рождения: не указана\nГруппа: без группы", "groups", ""},
		{"missing", "client_view:999", "Клиент не найден.", "groups", ""},
		{"database error", "client_view:1", "Не удалось загрузить клиента. Попробуйте ещё раз.", "groups", "ALTER TABLE clients RENAME TO broken"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := telegramTestDatabase(t)
			if _, err := pool.Exec(t.Context(), seed); err != nil {
				t.Fatal(err)
			}
			if tc.sql != "" {
				if _, err := pool.Exec(t.Context(), tc.sql); err != nil {
					t.Fatal(err)
				}
			}
			process := telegramTestSession(t, pool)
			got, keyboard, _ := process(&models.Update{CallbackQuery: &models.CallbackQuery{
				ID: "callback", From: models.User{ID: 7}, Data: tc.data,
				Message: models.MaybeInaccessibleMessage{Message: &models.Message{ID: 10, Chat: models.Chat{ID: 7, Type: models.ChatTypePrivate}}},
			}})
			if got != tc.want {
				t.Fatalf("text = %q, want %q", got, tc.want)
			}
			if !strings.Contains(keyboard, `"callback_data":"`+tc.back+`"`) {
				t.Fatalf("missing back to %q in %s", tc.back, keyboard)
			}
		})
	}
}
