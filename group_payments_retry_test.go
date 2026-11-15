package main

import (
	"io"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/go-telegram/bot/models"
	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/jackc/pgx/v5/pgxpool"
)

type lostPaymentReply struct {
	decoder  *pgproto3.Frontend
	pending  []byte
	inserted bool
	dropped  *atomic.Bool
}

func (r *lostPaymentReply) Read(p []byte) (int, error) {
	if len(r.pending) == 0 {
		message, err := r.decoder.Receive()
		if err != nil {
			return 0, err
		}
		if command, ok := message.(*pgproto3.CommandComplete); ok {
			r.inserted = strings.HasPrefix(string(command.CommandTag), "INSERT ")
		}
		// Hide the commit acknowledgement once, including across reconnects.
		if _, ready := message.(*pgproto3.ReadyForQuery); ready && r.inserted && r.dropped.CompareAndSwap(false, true) {
			return 0, io.ErrUnexpectedEOF
		}
		r.pending, err = message.Encode(nil)
		if err != nil {
			return 0, err
		}
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

func TestTelegramGroupPaymentLostReply(t *testing.T) {
	pool := telegramTestDatabase(t)
	if _, err := pool.Exec(t.Context(), `INSERT INTO groups (name) VALUES ('Утро');
		INSERT INTO clients (name, group_id) VALUES ('Иван', 1)`); err != nil {
		t.Fatal(err)
	}
	config := pool.Config()
	var dropped atomic.Bool
	config.ConnConfig.BuildFrontend = func(r io.Reader, w io.Writer) *pgproto3.Frontend {
		return pgproto3.NewFrontend(&lostPaymentReply{decoder: pgproto3.NewFrontend(r, nil), dropped: &dropped}, w)
	}
	faultPool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(faultPool.Close)
	process := telegramTestSession(t, faultPool, 6)
	message := &models.Message{From: &models.User{ID: 7}, Chat: models.Chat{ID: 7, Type: models.ChatTypePrivate}}
	var keyboard, preview string
	for _, tc := range []struct {
		data, input, want, sql string
	}{
		{"client_view:1", "", "", ""},
		{"payment:1", "", "", ""},
		{"", "3000", "", ""},
		{"", "01.2027", "", ""},
		{"", "15.11.2026", "", ""},
		{"payment_save", "", "", ""},
		{"payment_back", "", "Не удалось подтвердить сохранение оплаты.", ""},
		{"", "/start", "Не удалось подтвердить сохранение оплаты.", ""},
		{"payment_save", "", "Не удалось подтвердить сохранение оплаты.", "ALTER TABLE group_payments ADD CONSTRAINT verify_failure CHECK (amount_rub <> 3000) NOT VALID"},
		{"payment_save", "", "Оплата сохранена", "ALTER TABLE group_payments DROP CONSTRAINT verify_failure"},
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
			if strings.HasPrefix(data, "payment") {
				data = telegramTestCallback(t, keyboard, data)
			}
			update.CallbackQuery = &models.CallbackQuery{ID: "callback", From: *message.From, Data: data, Message: models.MaybeInaccessibleMessage{Message: message}}
		}
		got, next, _ := process(update)
		if tc.input == "15.11.2026" {
			preview = got
		}
		if strings.HasPrefix(tc.want, "Не удалось подтвердить") && (!strings.Contains(got, preview) || strings.Contains(next, "payment_back") || !strings.Contains(next, "Проверить сохранение")) {
			t.Fatalf("pending save must keep its preview and only offer verification: %q, %s", got, next)
		}
		if got != "" {
			keyboard = next
		}
		if tc.data == "payment_save" || tc.data == "payment_back" || tc.input == "/start" {
			if (tc.want == "" && got != "") || (tc.want != "" && !strings.Contains(got, tc.want)) {
				t.Fatalf("%s: got %q, want %q", tc.data, got, tc.want)
			}
			var count, amount int64
			if err := pool.QueryRow(t.Context(), "SELECT count(*), max(amount_rub) FROM group_payments").Scan(&count, &amount); err != nil || count != 1 || amount != 3000 {
				t.Fatalf("%s: payments=%d, amount=%d, error=%v", tc.data, count, amount, err)
			}
		}
	}
	if !dropped.Load() {
		t.Fatal("database commit acknowledgement was not dropped")
	}
}
