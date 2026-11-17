package main

import (
	"errors"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/xuri/excelize/v2"
)

func TestTelegramGroupExport(t *testing.T) {
	pool := telegramTestDatabase(t)
	if _, err := pool.Exec(t.Context(), `INSERT INTO groups (name, archived) VALUES ('Утро', TRUE), ('Вечер', FALSE);
		INSERT INTO clients (name, group_id, archived) VALUES ('=1+1', 2, FALSE), ('Анна', 2, TRUE);
		INSERT INTO group_payments (client_id, group_id, amount_rub, received_on, training_month) VALUES
		(1, 1, 3000, '2026-11-15', '2027-01-01'), (2, 1, 2000, '2027-02-15', '2027-01-01'),
		(1, 1, 1000, '2027-01-10', '2027-02-01'), (1, 2, 9900, '2027-01-10', '2027-01-01');
		INSERT INTO personal_payments (client_id, amount_rub, received_on, save_key) VALUES (1, 8888, '2027-01-10', 'personal')`); err != nil {
		t.Fatal(err)
	}
	var text, keyboard string
	var rows [][]string
	var edits, documents int
	var fail bool
	var amountKind, totalKind excelize.CellType
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		result := `true`
		switch strings.TrimPrefix(r.URL.Path, "/bot123:test-token/") {
		case "answerCallbackQuery", "deleteMessage":
		case "editMessageText":
			if r.FormValue("chat_id") != "7" || r.FormValue("message_id") != "10" {
				t.Fatalf("must keep the original screen: %v", r.Form)
			}
			edits++
			text, keyboard, result = r.FormValue("text"), r.FormValue("reply_markup"), `{"message_id":10}`
		case "sendDocument":
			documents++
			if fail {
				return nil, errors.New("offline")
			}
			part, header, err := r.FormFile("document")
			if err != nil {
				t.Fatal(err)
			}
			book, err := excelize.OpenReader(part)
			if closeErr := part.Close(); closeErr != nil {
				t.Fatal(closeErr)
			}
			if err != nil || r.FormValue("chat_id") != "7" || !strings.HasSuffix(header.Filename, ".xlsx") {
				t.Fatalf("invalid workbook upload: %v, %v", header, err)
			}
			rows, err = book.GetRows("Sheet1")
			if err != nil {
				t.Fatal(err)
			}
			amountKind, err = book.GetCellType("Sheet1", "B4")
			if err != nil {
				t.Fatal(err)
			}
			totalKind, err = book.GetCellType("Sheet1", "B"+strconv.Itoa(len(rows)))
			if err != nil {
				t.Fatal(err)
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
	for _, tc := range []struct {
		data, input, sql, period, total string
		count                           int
		fail                            bool
	}{
		{data: "group:1"}, {data: "summary:1"}, {data: "summary_month"}, {input: "01.2027"},
		{data: "summary_export", period: "01.2027", total: "5000", count: 6},
		{data: "summary_export", sql: "UPDATE group_payments SET amount_rub = 3500 WHERE id = 1; DELETE FROM group_payments WHERE id = 2", period: "01.2027", total: "3500", count: 5},
		{data: "summary_export", fail: true},
		{data: "summary_back"}, {data: "summary_month"}, {input: "01.2027"},
		{data: "summary_export", period: "01.2027", total: "3500", count: 5},
		{data: "summary_back"}, {data: "summary_all"},
		{data: "summary_export", period: "За всё время", total: "4500", count: 6},
		{data: "summary_back"}, {data: "summary_month"}, {input: "03.2027"},
		{data: "summary_export", period: "03.2027", total: "0", count: 4},
		{data: "summary_export", sql: "INSERT INTO group_payments (client_id, group_id, amount_rub, received_on, training_month) VALUES (1, 1, 9223372036854775807, '2027-03-01', '2027-03-01'), (2, 1, 9223372036854775807, '2027-03-02', '2027-03-01')", period: "03.2027", total: "18446744073709551614", count: 6},
		{data: "summary_back"}, {data: "group:1"},
	} {
		if tc.sql != "" {
			if _, err := pool.Exec(t.Context(), tc.sql); err != nil {
				t.Fatal(err)
			}
		}
		fail = tc.fail
		beforeEdits, beforeDocuments, beforeKeyboard := edits, documents, keyboard
		message := &models.Message{ID: 100, From: &models.User{ID: 7}, Chat: models.Chat{ID: 7, Type: models.ChatTypePrivate}, Text: tc.input}
		update := &models.Update{Message: message}
		if tc.data != "" {
			message.ID = 10
			data := tc.data
			if keyboard != "" {
				data = telegramTestCallback(t, keyboard, data)
			}
			update.CallbackQuery = &models.CallbackQuery{ID: "callback", From: *message.From, Data: data, Message: models.MaybeInaccessibleMessage{Message: message}}
		}
		b.ProcessUpdate(t.Context(), update)
		if tc.fail && (documents != beforeDocuments+1 || edits != beforeEdits+1 || !strings.Contains(text, "Не удалось выгрузить") || strings.Contains(keyboard, `"callback_data":"summary_export|`)) {
			t.Fatalf("upload failure must show an error and Back: %q, %s", text, keyboard)
		}
		if tc.total == "" {
			continue
		}
		if documents != beforeDocuments+1 || edits != beforeEdits || keyboard != beforeKeyboard || len(rows) != tc.count || rows[0][0] != "Группа: Утро" || rows[1][0] != "Период: "+tc.period || !reflect.DeepEqual(rows[len(rows)-1], []string{"Итого", tc.total}) {
			t.Fatalf("%s: rows=%v, edits=%d, documents=%d", tc.period, rows, edits-beforeEdits, documents-beforeDocuments)
		}
		if documents == 1 && !reflect.DeepEqual(rows[2:5], [][]string{{"Клиент", "Сумма (₽)", "Дата получения", "Группа", "Месяц занятий"}, {"=1+1", "3000", "15.11.2026", "Утро", "01.2027"}, {"Анна", "2000", "15.02.2027", "Утро", "01.2027"}}) {
			t.Fatalf("wrong exported rows: %v", rows)
		}
		if len(tc.total) > 15 && (rows[3][1] != "9223372036854775807" || (amountKind != excelize.CellTypeSharedString && amountKind != excelize.CellTypeInlineString) || (totalKind != excelize.CellTypeSharedString && totalKind != excelize.CellTypeInlineString)) {
			t.Fatalf("large amounts must stay exact text: %v", rows)
		}
	}
}
