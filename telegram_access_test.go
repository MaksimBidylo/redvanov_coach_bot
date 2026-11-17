package main

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func TestTelegramSharedAccess(t *testing.T) {
	pool := telegramTestDatabase(t)
	var active int64
	var calls int
	sends, keyboards := map[int64]int{}, map[int64]string{}
	var text string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		if active != 7 && active != 8 {
			t.Fatalf("unexpected request for user %d: %s", active, r.URL.Path)
		}
		result := `true`
		switch strings.TrimPrefix(r.URL.Path, "/bot123:test-token/") {
		case "answerCallbackQuery":
		case "deleteMessage":
			if r.FormValue("chat_id") != strconv.FormatInt(active, 10) || r.FormValue("message_id") != "100" {
				t.Fatalf("must delete this user's input: %v", r.Form)
			}
		case "sendMessage", "editMessageText":
			if r.FormValue("chat_id") != strconv.FormatInt(active, 10) {
				t.Fatalf("wrong destination for user %d: %v", active, r.Form)
			}
			if strings.HasSuffix(r.URL.Path, "/sendMessage") {
				sends[active]++
			} else if sends[active] != 1 || r.FormValue("message_id") != "10" {
				t.Fatalf("must edit this user's existing message: %v", r.Form)
			}
			text, keyboards[active] = r.FormValue("text"), r.FormValue("reply_markup")
			result = `{"message_id":10}`
		default:
			t.Fatalf("unexpected request: %s", r.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"ok":true,"result":` + result + `}`))}, nil
	})}
	b, err := newTelegramBot("7, 8", "123:test-token", pool, bot.WithHTTPClient(time.Second, client))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		owner             int64
		input, data, want string
	}{
		{7, "/start", "", "Главное меню"}, {8, "/start", "", "Главное меню"},
		{7, "", "groups", "Групп пока нет"}, {8, "", "groups", "Групп пока нет"},
		{7, "", "group_create", groupNamePrompt}, {8, "", "group_create", groupNamePrompt},
		{7, "Утро", "", "Группа создана."}, {8, "Вечер", "", "Группа создана."},
		{7, "", "groups", "Группы:"}, {8, "", "groups", "Группы:"},
		{9, "/start", "", ""}, {9, "", "groups", ""},
		{8, "/start", "", ""}, {8, "", "groups", ""},
	} {
		active, text = tc.owner, ""
		before := calls
		message := &models.Message{ID: 100, From: &models.User{ID: tc.owner}, Chat: models.Chat{ID: tc.owner, Type: models.ChatTypePrivate}, Text: tc.input}
		if tc.owner == 8 && tc.want == "" {
			message.Chat.Type = models.ChatTypeGroup
		}
		update := &models.Update{Message: message}
		if tc.data != "" {
			data := tc.data
			if tc.want != "" {
				data = telegramTestCallback(t, keyboards[tc.owner], data)
			}
			message.ID = 10
			update = &models.Update{CallbackQuery: &models.CallbackQuery{ID: "callback", From: *message.From, Data: data, Message: models.MaybeInaccessibleMessage{Message: message}}}
		}
		b.ProcessUpdate(t.Context(), update)
		if text != tc.want || tc.want == "" && calls != before {
			t.Fatalf("user %d, %q/%q: text=%q, calls=%d", tc.owner, tc.input, tc.data, text, calls-before)
		}
		if tc.want == "Группы:" {
			telegramTestCallback(t, keyboards[tc.owner], "group:1")
			telegramTestCallback(t, keyboards[tc.owner], "group:2")
		}
	}
	if sends[7] != 1 || sends[8] != 1 {
		t.Fatalf("must keep one bot message per user: %v", sends)
	}
}
