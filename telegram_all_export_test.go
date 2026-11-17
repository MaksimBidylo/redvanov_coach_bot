package main

import (
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/xuri/excelize/v2"
)

func TestTelegramAllExport(t *testing.T) {
	pool := telegramTestDatabase(t)
	if _, err := pool.Exec(t.Context(), `INSERT INTO groups (name, archived) VALUES ('Утро', TRUE), ('Вечер', FALSE);
		INSERT INTO clients (name, group_id, archived) VALUES ('Анна', 2, TRUE), ('Борис', NULL, FALSE);
		INSERT INTO group_payments (client_id, group_id, amount_rub, received_on, training_month) VALUES
		(1, 1, 3000, '2026-12-31', '2027-01-01'), (1, 2, 2000, '2027-02-01', '2027-02-01');
		INSERT INTO personal_payments (client_id, amount_rub, received_on, save_key) VALUES
		(1, 500, '2027-01-01', 'anna'), (2, 700, '2027-02-01', 'boris')`); err != nil {
		t.Fatal(err)
	}
	want := map[string][][]string{
		"Групповые": {{"Групповые оплаты"}, {"Период: За всё время"}, {"Клиент", "Сумма (₽)", "Дата получения", "Группа", "Месяц занятий"},
			{"Анна", "3000", "31.12.2026", "Утро", "01.2027"}, {"Анна", "2000", "01.02.2027", "Вечер", "02.2027"}, {"Итого", "5000"}},
		"Персональные": {{"Персональные оплаты"}, {"Период: За всё время"}, {"Клиент", "Сумма (₽)", "Дата получения"},
			{"Анна", "500", "01.01.2027"}, {"Борис", "700", "01.02.2027"}, {"Итого", "1200"}},
	}
	var text, keyboard string
	var uploads int
	var fail bool
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		result := `true`
		switch strings.TrimPrefix(r.URL.Path, "/bot123:test-token/") {
		case "answerCallbackQuery", "deleteMessage":
		case "sendMessage", "editMessageText":
			if r.FormValue("chat_id") != "7" || (uploads > 0 && !fail) || (strings.HasSuffix(r.URL.Path, "editMessageText") && r.FormValue("message_id") != "10") {
				t.Fatalf("wrong active screen: %v", r.Form)
			}
			text, keyboard, result = r.FormValue("text"), r.FormValue("reply_markup"), `{"message_id":10}`
		case "sendDocument":
			uploads++
			if fail {
				return nil, errors.New("offline")
			}
			part, header, err := r.FormFile("document")
			if err != nil {
				t.Fatal(err)
			}
			book, err := excelize.OpenReader(part)
			if err = errors.Join(err, part.Close()); err != nil || header.Filename != "all-payments.xlsx" || r.FormValue("chat_id") != "7" {
				t.Fatalf("invalid export: %v, %v", header, err)
			}
			if !reflect.DeepEqual(book.GetSheetList(), []string{"Групповые", "Персональные"}) {
				t.Fatalf("wrong sheets: %v", book.GetSheetList())
			}
			for sheet, expected := range want {
				if rows, err := book.GetRows(sheet); err != nil || !reflect.DeepEqual(rows, expected) {
					t.Fatalf("%s: rows=%v, error=%v", sheet, rows, err)
				}
			}
			if err := book.Close(); err != nil {
				t.Fatal(err)
			}
			result = `{"message_id":99}`
		default:
			t.Fatalf("unexpected request: %s", r.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"ok":true,"result":` + result + `}`))}, nil
	})}
	b, err := newTelegramBot("7", "123:test-token", pool, bot.WithHTTPClient(time.Second, client))
	if err != nil {
		t.Fatal(err)
	}
	message := &models.Message{ID: 100, From: &models.User{ID: 7}, Chat: models.Chat{ID: 7, Type: models.ChatTypePrivate}, Text: "/start"}
	b.ProcessUpdate(t.Context(), &models.Update{Message: message})
	if !strings.Contains(keyboard, "📥 Общая выгрузка") {
		t.Fatalf("missing main-menu export button: %s", keyboard)
	}
	for i, broken := range []bool{false, true, false} {
		fail, message.ID = broken, 10
		data := telegramTestCallback(t, keyboard, "export_all")
		b.ProcessUpdate(t.Context(), &models.Update{CallbackQuery: &models.CallbackQuery{ID: "callback", From: *message.From, Data: data, Message: models.MaybeInaccessibleMessage{Message: message}}})
		if uploads != i+1 || (broken && !strings.Contains(text, "Не удалось выгрузить")) {
			t.Fatalf("attempt %d: uploads=%d, text=%q", i, uploads, text)
		}
	}
}
