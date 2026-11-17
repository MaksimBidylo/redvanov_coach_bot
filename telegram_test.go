package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTelegramHandler(t *testing.T) {
	var sends, deletes int
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
		}
		if r.URL.Path == "/bottest-token/deleteMessage" {
			if r.FormValue("chat_id") != "7" || r.FormValue("message_id") != "100" {
				t.Errorf("must delete the user input: %v", r.Form)
			}
			deletes++
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"ok":true,"result":true}`))}, nil
		}
		method := map[int]string{0: "sendMessage", 1: "editMessageText", 2: "editMessageText", 3: "sendMessage"}[sends]
		if r.URL.Path != "/bottest-token/"+method {
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
		if r.FormValue("chat_id") != "7" || r.FormValue("text") != "Добро пожаловать!" {
			t.Errorf("unexpected message: %v", r.Form)
		}
		if method == "editMessageText" && r.FormValue("message_id") != "1" {
			t.Errorf("must edit the original message: %v", r.Form)
		}
		if !strings.Contains(r.FormValue("reply_markup"), `"callback_data":"groups"`) || !strings.Contains(r.FormValue("reply_markup"), `"callback_data":"ungrouped"`) || !strings.Contains(r.FormValue("reply_markup"), `"callback_data":"search"`) {
			t.Error("main menu must include groups, ungrouped clients and search buttons")
		}
		sends++
		if description := map[int]string{2: "message is not modified", 3: "message to edit not found"}[sends]; description != "" {
			return &http.Response{StatusCode: http.StatusBadRequest, Body: io.NopCloser(strings.NewReader(`{"ok":false,"error_code":400,"description":"` + description + `"}`))}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"ok":true,"result":{"message_id":` + strconv.Itoa(sends) + `}}`))}, nil
	})}
	b, err := newTelegramBot("7", "test-token", nil, bot.WithHTTPClient(time.Second, client))
	if err != nil {
		t.Fatal(err)
	}
	if sends != 0 {
		t.Fatal("initialization must not contact Telegram")
	}
	message := func(owner int64, chat models.ChatType, text string) *models.Message {
		return &models.Message{ID: 100, From: &models.User{ID: owner}, Chat: models.Chat{ID: 7, Type: chat}, Text: text}
	}
	for _, tc := range []struct {
		name             string
		message          *models.Message
		want, wantDelete int
	}{
		{"owner start", message(7, models.ChatTypePrivate, "/start"), 1, 1},
		{"same screen", message(7, models.ChatTypePrivate, "/start"), 1, 1},
		{"deleted screen", message(7, models.ChatTypePrivate, "/start"), 2, 1},
		{"other user", message(8, models.ChatTypePrivate, "/start"), 0, 0},
		{"group", message(7, models.ChatTypeGroup, "/start"), 0, 0},
		{"other command", message(7, models.ChatTypePrivate, "/help"), 0, 1},
		{"no message", nil, 0, 0},
		{"no sender", &models.Message{Chat: models.Chat{ID: 7, Type: models.ChatTypePrivate}, Text: "/start"}, 0, 0},
	} {
		before, deletesBefore := sends, deletes
		b.ProcessUpdate(context.Background(), &models.Update{Message: tc.message})
		if got := sends - before; got != tc.want {
			t.Fatalf("%s: sent %d messages, want %d", tc.name, got, tc.want)
		}
		if got := deletes - deletesBefore; got != tc.wantDelete {
			t.Fatalf("%s: deleted %d messages, want %d", tc.name, got, tc.wantDelete)
		}
	}
}

func TestTelegramSendFailure(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("offline")
	})}
	b, err := newTelegramBot("7", "test-token", nil, bot.WithHTTPClient(time.Second, client))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(previous)
	b.ProcessUpdate(context.Background(), &models.Update{Message: &models.Message{
		From: &models.User{ID: 7}, Chat: models.Chat{ID: 7, Type: models.ChatTypePrivate}, Text: "/start",
	}})
	if !strings.Contains(output.String(), "offline") || strings.Contains(output.String(), "test-token") {
		t.Fatalf("send error must be logged without the token: %q", output.String())
	}
}

func TestTelegramConfig(t *testing.T) {
	for _, config := range [][2]string{{"", "token"}, {"0", "token"}, {"-1", "token"}, {"invalid", "token"}, {"7,0", "token"}, {"7,invalid", "token"}, {"7,", "token"}, {"7", ""}, {"7", "   "}} {
		if b, err := newTelegramBot(config[0], config[1], nil); err == nil || b != nil {
			t.Fatal("invalid Telegram configuration accepted")
		}
	}
}

func telegramTestDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is required for the integration test")
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 1
	schema := "test_groups_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(t.Context(), "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := pool.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	if err := migrateDatabase(t.Context(), pool); err != nil {
		t.Fatal(err)
	}
	return pool
}

func telegramTestSession(t *testing.T, pool *pgxpool.Pool, failEdit ...int) func(*models.Update) (string, string, int) {
	t.Helper()
	var got, keyboard string
	var messageID, replyID, edits int
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		result := `true`
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
		}
		switch r.URL.Path {
		case "/bot123:test-token/answerCallbackQuery":
		case "/bot123:test-token/deleteMessage":
			if r.FormValue("chat_id") != "7" || r.FormValue("message_id") != "100" {
				t.Errorf("must delete only user input: %v", r.Form)
			}
		case "/bot123:test-token/editMessageText", "/bot123:test-token/sendMessage":
			if r.FormValue("message_id") != "" {
				edits++
				if len(failEdit) > 0 && edits == failEdit[0] {
					return nil, errors.New("offline")
				}
			}
			got, keyboard = r.FormValue("text"), r.FormValue("reply_markup")
			if r.FormValue("message_id") == "" {
				messageID++
				if messageID > 1 {
					t.Fatal("bot must reuse its existing message")
				}
				replyID = messageID
			} else {
				var err error
				replyID, err = strconv.Atoi(r.FormValue("message_id"))
				if err != nil {
					t.Fatal(err)
				}
			}
			result = `{"message_id":` + strconv.Itoa(replyID) + `}`
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"ok":true,"result":` + result + `}`))}, nil
	})}
	b, err := newTelegramBot("7", "123:test-token", pool, bot.WithHTTPClient(time.Second, client))
	if err != nil {
		t.Fatal(err)
	}
	return func(update *models.Update) (string, string, int) {
		got, keyboard = "", ""
		b.ProcessUpdate(t.Context(), update)
		return got, keyboard, replyID
	}
}

func TestTelegramMenu(t *testing.T) {
	const seedGroups = "INSERT INTO groups (name) VALUES ('Б вечер'), ('А утро')"
	const seedClients = seedGroups + `; INSERT INTO clients (name, birth_date, group_id) VALUES
		('Иван', NULL, 2), ('Анна', '2010-03-15', 2), ('Иван', '2000-04-20', 2),
		('Чужой', NULL, 1), ('Персональный', NULL, NULL)`
	private := &models.Message{ID: 10, Chat: models.Chat{ID: 7, Type: models.ChatTypePrivate}}
	group := &models.Message{Chat: models.Chat{Type: models.ChatTypeGroup}}
	for _, tc := range []struct {
		name    string
		owner   int64
		message *models.Message
		data    string
		want    string
		sql     string
	}{
		{"empty", 7, private, "groups", "Групп пока нет", ""},
		{"populated", 7, private, "groups", "Группы:", seedGroups},
		{"selected", 7, private, "group:2", "А утро\n\nКлиентов пока нет.", seedGroups},
		{"clients", 7, private, "group:2", "А утро\n\nКлиенты:", seedClients},
		{"clients database error", 7, private, "group:2", "А утро\n\nНе удалось загрузить клиентов. Попробуйте ещё раз.", seedGroups + "; ALTER TABLE clients RENAME TO broken"},
		{"ungrouped empty", 7, private, "ungrouped", "Клиенты без группы\n\nКлиентов пока нет.", ""},
		{"ungrouped clients", 7, private, "ungrouped", "Клиенты без группы\n\nКлиенты:", seedClients},
		{"ungrouped database error", 7, private, "ungrouped", "Клиенты без группы\n\nНе удалось загрузить клиентов. Попробуйте ещё раз.", "ALTER TABLE clients RENAME TO broken"},
		{"missing", 7, private, "group:999", "Группа не найдена.", ""},
		{"invalid id", 7, private, "group:invalid", "", ""},
		{"zero id", 7, private, "group:0", "", ""},
		{"overflow id", 7, private, "group:9223372036854775808", "", ""},
		{"other user", 8, private, "groups", "", ""},
		{"group", 7, group, "groups", "", ""},
		{"inaccessible", 7, nil, "groups", "", ""},
		{"unknown", 7, private, "unknown", "", ""},
		{"database error", 7, private, "groups", "Не удалось загрузить группы. Попробуйте ещё раз.", "ALTER TABLE groups RENAME COLUMN name TO broken"},
		{"group database error", 7, private, "group:2", "Не удалось загрузить группу. Попробуйте ещё раз.", "ALTER TABLE groups RENAME COLUMN name TO broken"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := telegramTestDatabase(t)
			process := telegramTestSession(t, pool)
			if tc.sql != "" {
				if _, err := pool.Exec(t.Context(), tc.sql); err != nil {
					t.Fatal(err)
				}
			}
			got, keyboard, _ := process(&models.Update{CallbackQuery: &models.CallbackQuery{
				ID: "callback", From: models.User{ID: tc.owner}, Data: tc.data,
				Message: models.MaybeInaccessibleMessage{Message: tc.message},
			}})
			if got != tc.want {
				t.Fatalf("text = %q, want %q", got, tc.want)
			}
			markup := `"callback_data":"menu"`
			if strings.HasPrefix(tc.data, "group:") {
				markup = `"callback_data":"groups"`
			}
			if tc.want != "" && !strings.Contains(keyboard, markup) {
				t.Fatalf("missing %q button in %s", markup, keyboard)
			}
			if tc.name == "clients database error" && !strings.Contains(keyboard, `"callback_data":"client:2"`) {
				t.Fatalf("missing add client button after list error: %s", keyboard)
			}
			if tc.name == "clients" && !strings.HasPrefix(keyboard, `{"inline_keyboard":[[{"text":"Анна (15.03.2010)","callback_data":"client_view:2"}],[{"text":"Иван","callback_data":"client_view:1"}],[{"text":"Иван (20.04.2000)","callback_data":"client_view:3"}]`) {
				t.Fatalf("unexpected client buttons: %s", keyboard)
			}
			if tc.name == "ungrouped clients" && !strings.HasPrefix(keyboard, `{"inline_keyboard":[[{"text":"Персональный","callback_data":"client_view:5"}],[{"text":"➕ Добавить клиента","callback_data":"client:0"}]`) {
				t.Fatalf("unexpected ungrouped client buttons: %s", keyboard)
			}
			if tc.name == "populated" && !strings.HasPrefix(keyboard, `{"inline_keyboard":[[{"text":"А утро","callback_data":"group:2"}],[{"text":"Б вечер","callback_data":"group:1"}]`) {
				t.Fatalf("unexpected group buttons: %s", keyboard)
			}
		})
	}
}

func TestTelegramGroupCreation(t *testing.T) {
	pool := telegramTestDatabase(t)
	process := telegramTestSession(t, pool)
	var previousKeyboard string
	for _, tc := range []struct {
		name, text, data string
		owner            int64
		want             string
		sql              string
	}{
		{"idle", "Лишняя", "", 7, "", ""},
		{"begin", "", "group_create", 7, "Введите название группы.", ""},
		{"invalid callback", "", "group:invalid", 7, "", ""},
		{"other user", "Чужая", "", 8, "", ""},
		{"empty", " \n ", "", 7, "Название не должно быть пустым.\n" + groupNamePrompt, ""},
		{"created", "  Взрослая утро  ", "", 7, "Группа создана.", ""},
		{"completed", "Лишняя", "", 7, "", ""},
		{"begin again", "", "group_create", 7, groupNamePrompt, ""},
		{"duplicate", "ВЗРОСЛАЯ УТРО", "", 7, "Группа с таким названием уже есть.\n" + groupNamePrompt, ""},
		{"retry", "Детская", "", 7, "Группа создана.", ""},
		{"begin cancel", "", "group_create", 7, groupNamePrompt, ""},
		{"cancel", "/start", "", 7, "Добро пожаловать!", ""},
		{"after cancel", "Лишняя", "", 7, "", ""},
		{"begin back", "", "group_create", 7, groupNamePrompt, ""},
		{"back", "", "groups", 7, "Группы:", ""},
		{"after back", "Лишняя", "", 7, "", ""},
		{"begin menu", "", "group_create", 7, groupNamePrompt, ""},
		{"menu", "", "menu", 7, "Добро пожаловать!", ""},
		{"after menu", "Лишняя", "", 7, "", ""},
		{"begin error", "", "group_create", 7, groupNamePrompt, ""},
		{"database error", "Выходные", "", 7, "Не удалось создать группу. Попробуйте ещё раз.\n" + groupNamePrompt, "ALTER TABLE groups RENAME COLUMN name TO broken"},
		{"recovery", "Выходные", "", 7, "Группа создана.", "ALTER TABLE groups RENAME COLUMN broken TO name"},
	} {
		if tc.sql != "" {
			if _, err := pool.Exec(t.Context(), tc.sql); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
		}
		message := &models.Message{
			ID:   100,
			From: &models.User{ID: tc.owner}, Chat: models.Chat{ID: 7, Type: models.ChatTypePrivate},
			Text: tc.text,
		}
		update := &models.Update{Message: message}
		if tc.data != "" {
			message.ID = 10
			data := tc.data
			if tc.name == "back" {
				data = telegramTestCallback(t, previousKeyboard, "groups")
			}
			update = &models.Update{CallbackQuery: &models.CallbackQuery{
				ID: "callback", From: *message.From, Data: data,
				Message: models.MaybeInaccessibleMessage{Message: message},
			}}
		}
		got, keyboard, _ := process(update)
		if got != "" {
			previousKeyboard = keyboard
		}
		if got != tc.want {
			t.Fatalf("%s: text = %q, want %q", tc.name, got, tc.want)
		}
		if strings.Contains(keyboard, `"force_reply":true`) {
			t.Fatalf("%s: name input must not force a reply", tc.name)
		}
		if strings.Contains(tc.want, groupNamePrompt) && keyboard != `{"inline_keyboard":[[{"text":"⬅️ Назад","callback_data":"groups"}]]}` {
			t.Fatalf("%s: name input must have only Back: %s", tc.name, keyboard)
		}
		button := "groups"
		if tc.data == "groups" {
			button = "menu"
		}
		if tc.want != "" && !strings.Contains(keyboard, `"callback_data":"`+button+`"`) {
			t.Fatalf("%s: missing return button", tc.name)
		}
	}
	groups, err := listGroups(t.Context(), pool, false)
	if err != nil || len(groups) != 3 || groups[0].Name != "Взрослая утро" || groups[1].Name != "Выходные" || groups[2].Name != "Детская" {
		t.Fatalf("saved groups = %v, error = %v", groups, err)
	}
}

func telegramTestCallback(t *testing.T, keyboard, data string) string {
	t.Helper()
	var markup models.InlineKeyboardMarkup
	if err := json.Unmarshal([]byte(keyboard), &markup); err != nil {
		t.Fatal(err)
	}
	for _, button := range slices.Concat(markup.InlineKeyboard...) {
		if base, _, _ := strings.Cut(button.CallbackData, "|"); base == data {
			return button.CallbackData
		}
	}
	t.Fatalf("missing %q button in %s", data, keyboard)
	return ""
}
