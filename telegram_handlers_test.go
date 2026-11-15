package main

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func TestTelegramInputCleanup(t *testing.T) {
	for _, tc := range []struct {
		name, setup, text, fail, want string
	}{
		{"idle", "", "Лишний текст", "", "deleteMessage"},
		{"start", "", "/start", "", "editMessageText,deleteMessage"},
		{"name", "name", "Иван", "", "editMessageText,deleteMessage"},
		{"invalid date", "birth", "31.02.2010", "", "editMessageText,deleteMessage"},
		{"empty group", "group", " ", "", "editMessageText,deleteMessage"},
		{"empty search", "search", " ", "", "editMessageText,deleteMessage"},
		{"completed search", "search ready", "Иван", "", "deleteMessage"},
		{"payment preview", "payment preview", "Лишний текст", "", "deleteMessage"},
		{"reply error", "", "/start", "editMessageText", "editMessageText"},
		{"search reply error", "search", " ", "editMessageText", "editMessageText"},
		{"delete error", "", "/start", "deleteMessage", "editMessageText,deleteMessage"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &telegramHandler{ownerID: 7, messageID: 100}
			message := &models.Message{ID: 101, From: &models.User{ID: 7}, Chat: models.Chat{ID: 7, Type: models.ChatTypePrivate}, Text: tc.text}
			switch tc.setup {
			case "name":
				h.clientDraft = &clientDraft{}
			case "birth":
				h.clientDraft = &clientDraft{name: "Иван"}
			case "group":
				h.waitingForGroupName.Store(true)
			case "search":
				h.search = &clientSearch{}
			case "search ready":
				h.search = &clientSearch{query: "Иван", messageID: 100}
			case "payment preview":
				h.paymentDraft = &paymentDraft{step: 4}
			}
			var calls []string
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if err := r.ParseMultipartForm(1 << 20); err != nil {
					t.Fatal(err)
				}
				method := strings.TrimPrefix(r.URL.Path, "/bottest-token/")
				calls = append(calls, method)
				id, result := "100", `{"message_id":100}`
				if method == "deleteMessage" {
					id, result = "101", "true"
				}
				if r.FormValue("chat_id") != "7" || r.FormValue("message_id") != id {
					t.Errorf("wrong %s target: %v", method, r.Form)
				}
				if method == tc.fail {
					return nil, errors.New("offline")
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"ok":true,"result":` + result + `}`))}, nil
			})}
			b, err := newTelegramBot("7", "test-token", nil, bot.WithHTTPClient(time.Second, client))
			if err != nil {
				t.Fatal(err)
			}
			h.handleMessage(t.Context(), b, message)
			if got := strings.Join(calls, ","); got != tc.want {
				t.Fatalf("calls = %q, want %q", got, tc.want)
			}
		})
	}
}
