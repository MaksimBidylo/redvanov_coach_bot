package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTelegramHandler(t *testing.T) {
	var sends atomic.Int64
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
		sends.Add(1)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"ok":true,"result":{}}`))}, nil
	})}
	b, err := newTelegramBot("7", "test-token", nil, bot.WithHTTPClient(time.Second, client), bot.WithNotAsyncHandlers())
	if err != nil {
		t.Fatal(err)
	}
	if sends.Load() != 0 {
		t.Fatal("initialization must not contact Telegram")
	}
	message := func(owner int64, chat models.ChatType, text string) *models.Message {
		return &models.Message{From: &models.User{ID: owner}, Chat: models.Chat{ID: 7, Type: chat}, Text: text}
	}
	for _, tc := range []struct {
		name    string
		message *models.Message
		want    int64
	}{
		{"owner start", message(7, models.ChatTypePrivate, "/start"), 1},
		{"other user", message(8, models.ChatTypePrivate, "/start"), 0},
		{"group", message(7, models.ChatTypeGroup, "/start"), 0},
		{"other command", message(7, models.ChatTypePrivate, "/help"), 0},
		{"no message", nil, 0},
		{"no sender", &models.Message{Chat: models.Chat{ID: 7, Type: models.ChatTypePrivate}, Text: "/start"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := sends.Load()
			b.ProcessUpdate(context.Background(), &models.Update{Message: tc.message})
			if got := sends.Load() - before; got != tc.want {
				t.Fatalf("sent %d messages, want %d", got, tc.want)
			}
		})
	}
	var wg sync.WaitGroup
	before := sends.Load()
	for range 32 {
		wg.Go(func() {
			b.ProcessUpdate(context.Background(), &models.Update{Message: message(7, models.ChatTypePrivate, "/start")})
		})
	}
	wg.Wait()
	if got := sends.Load() - before; got != 32 {
		t.Fatalf("concurrent handlers sent %d messages, want 32", got)
	}
}

func TestTelegramSendFailure(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("offline")
	})}
	b, err := newTelegramBot("7", "test-token", nil, bot.WithHTTPClient(time.Second, client), bot.WithNotAsyncHandlers())
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

func TestTelegramMenu(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is required for the integration test")
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(t.Context(), "CREATE TEMP TABLE groups (name TEXT)"); err != nil {
		t.Fatal(err)
	}
	var got, keyboard string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/bottest-token/answerCallbackQuery":
		case "/bottest-token/editMessageText":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatal(err)
			}
			got, keyboard = r.FormValue("text"), r.FormValue("reply_markup")
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"ok":true,"result":true}`))}, nil
	})}
	b, err := newTelegramBot("7", "test-token", pool, bot.WithHTTPClient(time.Second, client), bot.WithNotAsyncHandlers())
	if err != nil {
		t.Fatal(err)
	}
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
		{"back", 7, private, "menu", "Добро пожаловать!", ""},
		{"populated", 7, private, "groups", "Группы:\nА утро\nБ вечер", "INSERT INTO groups (name) VALUES ('Б вечер'), ('А утро')"},
		{"other user", 8, private, "groups", "", ""},
		{"group", 7, group, "groups", "", ""},
		{"inaccessible", 7, nil, "groups", "", ""},
		{"unknown", 7, private, "unknown", "", ""},
		{"database error", 7, private, "groups", "Не удалось загрузить группы. Попробуйте ещё раз.", "ALTER TABLE pg_temp.groups RENAME COLUMN name TO broken"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.sql != "" {
				if _, err := pool.Exec(t.Context(), tc.sql); err != nil {
					t.Fatal(err)
				}
			}
			got, keyboard = "", ""
			want, button := tc.want, "menu"
			if tc.data == "menu" {
				button = "groups"
			}
			b.ProcessUpdate(t.Context(), &models.Update{CallbackQuery: &models.CallbackQuery{
				ID: "callback", From: models.User{ID: tc.owner}, Data: tc.data,
				Message: models.MaybeInaccessibleMessage{Message: tc.message},
			}})
			if got != want {
				t.Fatalf("text = %q, want %q", got, want)
			}
			if want != "" && !strings.Contains(keyboard, `"callback_data":"`+button+`"`) {
				t.Fatalf("missing %q button in %s", button, keyboard)
			}
		})
	}
}
