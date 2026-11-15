package main

import (
	"strings"
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestTelegramGroupRename(t *testing.T) {
	pool := telegramTestDatabase(t)
	if _, err := pool.Exec(t.Context(), `INSERT INTO groups (name) VALUES ('Утро'), ('Вечер'), ('Детская');
		INSERT INTO clients (name, birth_date, group_id) VALUES ('Иван', '2010-03-15', 1)`); err != nil {
		t.Fatal(err)
	}
	process := telegramTestSession(t, pool, 7)
	message := &models.Message{From: &models.User{ID: 7}, Chat: models.Chat{ID: 7, Type: models.ChatTypePrivate}}
	var keyboard, oldRename string
	name := "Утро"
	for _, tc := range []struct{ data, input, want, back, sql, name string }{
		{"group:1", "", "Утро\n\n", "rename:1", "", ""},
		{"rename:1", "", groupRenamePrompt, "group:1", "", ""},
		{"", "  ", "Название не должно быть пустым.\n" + groupRenamePrompt, "group:1", "", ""},
		{"", " вечер ", "Группа с таким названием уже есть.\n" + groupRenamePrompt, "group:1", "", ""},
		{"", "  Дети  ", "Дети\n\n", "rename:1", "", "Дети"},
		{"rename:1", "", groupRenamePrompt, "group:1", "", ""},
		{"", "дети", "", "", "", "дети"},
		{"", "дети", "дети\n\n", "rename:1", "", ""},
		{"rename:1", "", groupRenamePrompt, "group:1", "", ""},
		{"group:1", "", "дети\n\n", "rename:1", "", ""},
		{"", "Не сохранять", "", "", "", ""},
		{"rename:1", "", groupRenamePrompt, "group:1", "", ""},
		{"", "/start", "Добро пожаловать!", "groups", "", ""},
		{"old", "", "", "", "", ""},
		{"", "Не сохранять", "", "", "", ""},
		{"group:1", "", "дети\n\n", "rename:1", "", ""},
		{"rename:1", "", groupRenamePrompt, "group:1", "", ""},
		{"", "Ошибка", "Не удалось изменить название группы. Попробуйте ещё раз.\n" + groupRenamePrompt, "group:1", "ALTER TABLE groups ADD CONSTRAINT rename_failure CHECK (name <> 'Ошибка')", ""},
		{"", "Утро2", "Утро2\n\n", "rename:1", "ALTER TABLE groups DROP CONSTRAINT rename_failure", "Утро2"},
		{"group:3", "", "Детская\n\n", "rename:3", "", ""},
		{"rename:3", "", groupRenamePrompt, "group:3", "", ""},
		{"", "Исчезла", "Группа не найдена.", "groups", "DELETE FROM groups WHERE id = 3", ""},
		{"", "Не сохранять", "", "", "", ""},
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
			if strings.HasPrefix(data, "rename:") {
				data = telegramTestCallback(t, keyboard, data)
			} else if data == "old" {
				data = oldRename
			}
			update.CallbackQuery = &models.CallbackQuery{ID: "callback", From: *message.From, Data: data, Message: models.MaybeInaccessibleMessage{Message: message}}
		}
		got, next, _ := process(update)
		if (tc.want == "" && got != "") || (tc.want != "" && !strings.HasPrefix(got, tc.want)) {
			t.Fatalf("%s/%q: got %q, want %q", tc.data, tc.input, got, tc.want)
		}
		if got != "" {
			keyboard = next
		}
		if tc.back != "" {
			telegramTestCallback(t, keyboard, tc.back)
		}
		if oldRename == "" {
			oldRename = telegramTestCallback(t, keyboard, "rename:1")
		}
		if tc.name != "" {
			name = tc.name
		}
		c, err := getClient(t.Context(), pool, 1)
		if err != nil || c.Name != "Иван" || c.BirthDate == nil || c.BirthDate.Format("02.01.2006") != "15.03.2010" || c.GroupID == nil || *c.GroupID != 1 || c.GroupName == nil || *c.GroupName != name {
			t.Fatalf("%s/%q: client=%+v, want group 1/%q, error=%v", tc.data, tc.input, c, name, err)
		}
		other, err := getGroupName(t.Context(), pool, 2)
		if err != nil || other != "Вечер" {
			t.Fatalf("other group changed: %q, error=%v", other, err)
		}
	}
}
