package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
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
	var sends int
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/bottest-token/sendMessage" {
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
		}
		if r.FormValue("chat_id") != "7" || r.FormValue("text") != "Добро пожаловать!" {
			t.Errorf("unexpected message: %v", r.Form)
		}
		if !strings.Contains(r.FormValue("reply_markup"), `"callback_data":"groups"`) {
			t.Error("main menu must include the groups button")
		}
		sends++
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"ok":true,"result":{}}`))}, nil
	})}
	b, err := newTelegramBot("7", "test-token", nil, bot.WithHTTPClient(time.Second, client))
	if err != nil {
		t.Fatal(err)
	}
	if sends != 0 {
		t.Fatal("initialization must not contact Telegram")
	}
	message := func(owner int64, chat models.ChatType, text string) *models.Message {
		return &models.Message{From: &models.User{ID: owner}, Chat: models.Chat{ID: 7, Type: chat}, Text: text}
	}
	for _, tc := range []struct {
		name    string
		message *models.Message
		want    int
	}{
		{"owner start", message(7, models.ChatTypePrivate, "/start"), 1},
		{"other user", message(8, models.ChatTypePrivate, "/start"), 0},
		{"group", message(7, models.ChatTypeGroup, "/start"), 0},
		{"other command", message(7, models.ChatTypePrivate, "/help"), 0},
		{"no message", nil, 0},
		{"no sender", &models.Message{Chat: models.Chat{ID: 7, Type: models.ChatTypePrivate}, Text: "/start"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := sends
			b.ProcessUpdate(context.Background(), &models.Update{Message: tc.message})
			if got := sends - before; got != tc.want {
				t.Fatalf("sent %d messages, want %d", got, tc.want)
			}
		})
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
	for _, config := range [][2]string{{"", "token"}, {"0", "token"}, {"-1", "token"}, {"invalid", "token"}, {"7", ""}, {"7", "   "}} {
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

func telegramTestSession(t *testing.T, pool *pgxpool.Pool) func(*models.Update) (string, string) {
	t.Helper()
	var got, keyboard string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		result := `true`
		switch r.URL.Path {
		case "/bot123:test-token/answerCallbackQuery":
		case "/bot123:test-token/editMessageText", "/bot123:test-token/sendMessage":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
			}
			got, keyboard = r.FormValue("text"), r.FormValue("reply_markup")
			result = `{}`
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"ok":true,"result":` + result + `}`))}, nil
	})}
	b, err := newTelegramBot("7", "123:test-token", pool, bot.WithHTTPClient(time.Second, client))
	if err != nil {
		t.Fatal(err)
	}
	return func(update *models.Update) (string, string) {
		got, keyboard = "", ""
		b.ProcessUpdate(t.Context(), update)
		return got, keyboard
	}
}

func TestTelegramMenu(t *testing.T) {
	pool := telegramTestDatabase(t)
	process := telegramTestSession(t, pool)
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
		{"populated", 7, private, "groups", "Группы:\nА утро\nБ вечер", "INSERT INTO groups (name) VALUES ('Б вечер'), ('А утро')"},
		{"other user", 8, private, "groups", "", ""},
		{"group", 7, group, "groups", "", ""},
		{"inaccessible", 7, nil, "groups", "", ""},
		{"unknown", 7, private, "unknown", "", ""},
		{"database error", 7, private, "groups", "Не удалось загрузить группы. Попробуйте ещё раз.", "ALTER TABLE groups RENAME COLUMN name TO broken"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.sql != "" {
				if _, err := pool.Exec(t.Context(), tc.sql); err != nil {
					t.Fatal(err)
				}
			}
			got, keyboard := process(&models.Update{CallbackQuery: &models.CallbackQuery{
				ID: "callback", From: models.User{ID: tc.owner}, Data: tc.data,
				Message: models.MaybeInaccessibleMessage{Message: tc.message},
			}})
			if got != tc.want {
				t.Fatalf("text = %q, want %q", got, tc.want)
			}
			markup := `"callback_data":"menu"`
			if tc.want != "" && !strings.Contains(keyboard, markup) {
				t.Fatalf("missing %q button in %s", markup, keyboard)
			}
		})
	}
}

func TestTelegramGroupCreation(t *testing.T) {
	pool := telegramTestDatabase(t)
	process := telegramTestSession(t, pool)
	for _, tc := range []struct {
		name, text, data string
		owner            int64
		want             string
		sql              string
	}{
		{"idle", "Лишняя", "", 7, "", ""},
		{"begin", "", "group_create", 7, groupNamePrompt, ""},
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
		{"back", "", "groups", 7, "Группы:\nВзрослая утро\nДетская", ""},
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
			From: &models.User{ID: tc.owner}, Chat: models.Chat{ID: 7, Type: models.ChatTypePrivate},
			Text: tc.text,
		}
		update := &models.Update{Message: message}
		if tc.data != "" {
			update = &models.Update{CallbackQuery: &models.CallbackQuery{
				ID: "callback", From: *message.From, Data: tc.data,
				Message: models.MaybeInaccessibleMessage{Message: message},
			}}
		}
		got, keyboard := process(update)
		if got != tc.want {
			t.Fatalf("%s: text = %q, want %q", tc.name, got, tc.want)
		}
		if strings.Contains(keyboard, `"force_reply":true`) {
			t.Fatalf("%s: name input must not force a reply", tc.name)
		}
		button := "groups"
		if tc.data == "groups" {
			button = "menu"
		}
		if tc.want != "" && !strings.Contains(keyboard, `"callback_data":"`+button+`"`) {
			t.Fatalf("%s: missing return button", tc.name)
		}
	}
	names, err := listGroupNames(t.Context(), pool)
	if err != nil || len(names) != 3 || names[0] != "Взрослая утро" || names[1] != "Выходные" || names[2] != "Детская" {
		t.Fatalf("saved groups = %v, error = %v", names, err)
	}
}
