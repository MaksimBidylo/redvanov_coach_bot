package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
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
		sends.Add(1)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"ok":true,"result":{}}`))}, nil
	})}
	b, err := newTelegramBot("7", "test-token", bot.WithHTTPClient(time.Second, client), bot.WithNotAsyncHandlers())
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
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.ProcessUpdate(context.Background(), &models.Update{Message: message(7, models.ChatTypePrivate, "/start")})
		}()
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
	b, err := newTelegramBot("7", "test-token", bot.WithHTTPClient(time.Second, client), bot.WithNotAsyncHandlers())
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
		if b, err := newTelegramBot(config[0], config[1]); err == nil || b != nil {
			t.Fatal("invalid Telegram configuration accepted")
		}
	}
}
