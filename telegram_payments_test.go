package main

import (
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
)

func TestTelegramGroupPayment(t *testing.T) {
	pool := telegramTestDatabase(t)
	if _, err := pool.Exec(t.Context(), `INSERT INTO groups (name) VALUES ('Утро'); INSERT INTO clients (name, group_id) VALUES ('Иван', 1)`); err != nil {
		t.Fatal(err)
	}
	process := telegramTestSession(t, pool, 19)
	message := &models.Message{From: &models.User{ID: 7}, Chat: models.Chat{ID: 7, Type: models.ChatTypePrivate}}
	moscow := time.FixedZone("MSK", 3*60*60)
	var keyboard, save, monthBefore, monthAfter string
	for _, tc := range []struct {
		data, input, want, sql string
		count                  int
	}{
		{"client_view:1", "", "Иван\n", "", 0},
		{"payment:1", "", "Введите сумму", "", 0},
		{"payment_back", "", "Иван\n", "", 0},
		{"", "999", "", "", 0},
		{"payment:1", "", "Введите сумму", "", 0},
		{"", "0", "положительную целую сумму", "", 0},
		{"", "9223372036854775808", "положительную целую сумму", "", 0},
		{"", "3000", "Введите месяц", "", 0},
		{"payment_current", "", "Введите дату получения", "", 0},
		{"payment_current", "", "Проверьте оплату", "", 0},
		{"payment_back", "", "Введите дату получения", "", 0},
		{"payment_back", "", "Введите месяц", "", 0},
		{"", "13.2026", "Некорректная дата", "", 0},
		{"", "01.2027", "Введите дату получения", "", 0},
		{"", "31.02.2026", "Некорректная дата", "", 0},
		{"", "15.11.2026", "Клиент: Иван\nГруппа: Утро\nСумма: 3000 ₽\nМесяц занятий: 01.2027\nДата получения денег: 15.11.2026", "", 0},
		{"payment_back", "", "Введите дату получения", "", 0},
		{"", "16.11.2026", "Дата получения денег: 16.11.2026", "", 0},
		{"payment_save", "", "Не удалось сохранить оплату", "UPDATE clients SET group_id = NULL WHERE id = 1", 0},
		{"payment_save", "", "", "UPDATE clients SET group_id = 1 WHERE id = 1", 1},
		{"payment_save", "", "Оплата сохранена", "", 1},
		{"old", "", "", "", 1},
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
			if data == "old" {
				data = save
			} else if strings.HasPrefix(data, "payment") {
				data = telegramTestCallback(t, keyboard, data)
				if tc.data == "payment_save" {
					save = data
				}
			}
			update.CallbackQuery = &models.CallbackQuery{ID: "callback", From: *message.From, Data: data, Message: models.MaybeInaccessibleMessage{Message: message}}
		}
		before := time.Now().In(moscow)
		got, next, _ := process(update)
		after := time.Now().In(moscow)
		if (tc.want == "" && got != "") || (tc.want != "" && !strings.Contains(got, tc.want)) {
			t.Fatalf("%s/%q: got %q, want %q", tc.data, tc.input, got, tc.want)
		}
		if tc.data == "payment_current" {
			if monthBefore == "" {
				monthBefore, monthAfter = before.Format("01.2006"), after.Format("01.2006")
			} else {
				if !strings.Contains(got, "Месяц занятий: "+monthBefore) && !strings.Contains(got, "Месяц занятий: "+monthAfter) {
					t.Fatalf("current month must use Moscow time: %q", got)
				}
				if !strings.Contains(got, "Дата получения денег: "+before.Format("02.01.2006")) && !strings.Contains(got, "Дата получения денег: "+after.Format("02.01.2006")) {
					t.Fatalf("today must use Moscow time: %q", got)
				}
			}
		}
		if got != "" {
			keyboard = next
		}
		var count int
		if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM group_payments").Scan(&count); err != nil || count != tc.count {
			t.Fatalf("%s/%q: payments=%d, want %d, error=%v", tc.data, tc.input, count, tc.count, err)
		}
	}
	var clientID, groupID, amount int64
	var month, received string
	err := pool.QueryRow(t.Context(), "SELECT client_id, group_id, amount_rub, training_month::text, received_on::text FROM group_payments").Scan(&clientID, &groupID, &amount, &month, &received)
	if err != nil || clientID != 1 || groupID != 1 || amount != 3000 || month != "2027-01-01" || received != "2026-11-16" {
		t.Fatalf("saved payment=%d/%d/%d/%s/%s, error=%v", clientID, groupID, amount, month, received, err)
	}
}
