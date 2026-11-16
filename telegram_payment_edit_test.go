package main

import (
	"strings"
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestTelegramPaymentEdit(t *testing.T) {
	pool := telegramTestDatabase(t)
	exec := func(sql string) {
		t.Helper()
		if _, err := pool.Exec(t.Context(), sql); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO groups (name) VALUES ('Утро'), ('Вечер');
		INSERT INTO clients (name, group_id) VALUES ('Иван', 2), ('Другой', NULL);
		UPDATE groups SET archived = TRUE WHERE id = 1;
		INSERT INTO group_payments (client_id, group_id, amount_rub, received_on, training_month, save_key)
		VALUES (1, 1, 3000, '2026-11-15', '2027-01-01', 'group-own');
		INSERT INTO personal_payments (client_id, amount_rub, received_on, save_key)
		VALUES (1, 1000, '2026-11-14', 'own'), (2, 9999, '2026-11-13', 'other')`)
	process := telegramTestSession(t, pool, 13)
	message := &models.Message{From: &models.User{ID: 7}, Chat: models.Chat{ID: 7, Type: models.ChatTypePrivate}}
	const group = "Групповая оплата\nСумма: 3000 ₽\nДата получения денег: 15.11.2026\nГруппа: Утро\nМесяц занятий: 01.2027"
	const personal = "Персональная оплата\nСумма: 1000 ₽\nДата получения денег: 14.11.2026"
	var keyboard, detail string
	step := func(data, input, want string) {
		t.Helper()
		message.ID, message.Text = 100, input
		update := &models.Update{Message: message}
		if data != "" {
			message.ID = 10
			if strings.HasPrefix(data, "ledger") && !strings.Contains(data, "|") {
				data = telegramTestCallback(t, keyboard, data)
			}
			update = &models.Update{CallbackQuery: &models.CallbackQuery{ID: "callback", From: *message.From, Data: data, Message: models.MaybeInaccessibleMessage{Message: message}}}
		}
		got, next, _ := process(update)
		if (want == "" && got != "") || (want != "" && !strings.Contains(got, want)) || (strings.HasPrefix(data, "ledger_view:") && got != want) {
			t.Fatalf("%s/%q: got %q, want %q", data, input, got, want)
		}
		if got != "" {
			keyboard = next
		}
		if strings.HasPrefix(data, "ledger_view:") {
			detail = got
		}
		if strings.HasPrefix(data, "ledger_edit:") && strings.HasPrefix(detail, "Персональная") == strings.Contains(keyboard, "ledger_edit_month") {
			t.Fatalf("wrong month edit availability: %s", keyboard)
		}
		if strings.Contains(got, "Проверьте изменение:") && (!strings.Contains(got, detail) || !strings.Contains(got, input)) {
			t.Fatalf("incomplete change preview: %q", got)
		}
	}
	wantGroup, wantPersonal := "(1,1,1,3000,2026-11-15,2027-01-01,group-own)", "(1,1,,1000,2026-11-14,,own)"
	checkSaved := func() {
		t.Helper()
		var got string
		err := pool.QueryRow(t.Context(), `SELECT string_agg(payment, E'\n' ORDER BY kind, id) FROM (
			SELECT 0 AS kind, id, row(id, client_id, group_id, amount_rub, received_on, training_month, save_key)::text AS payment FROM group_payments
			UNION ALL SELECT 1, id, row(id, client_id, NULL::bigint, amount_rub, received_on, NULL::date, save_key)::text FROM personal_payments) payments`).Scan(&got)
		want := wantGroup + "\n" + wantPersonal + "\n(2,2,,9999,2026-11-13,,other)"
		if err != nil || got != want {
			t.Fatalf("saved payments = %q, want %q, error=%v", got, want, err)
		}
	}
	step("client_view:1", "", "Иван\n")
	step("ledger:1", "", "Оплаты:")
	step("ledger_view:0", "", group)
	step("ledger_edit:0", "", "оплат")
	step("ledger_edit_amount", "", "Введите сумму оплаты.")
	step("", "0", "Введите положительную целую сумму.")
	step("", "3500", "Проверьте изменение:")
	staleSave := telegramTestCallback(t, keyboard, "ledger_edit_save")
	step("ledger_edit_back", "", group)
	step(staleSave, "", "")
	checkSaved()
	step("ledger_edit:0", "", "оплат")
	step("ledger_edit_amount", "", "Введите сумму оплаты.")
	step("", "3500", "Проверьте изменение:")
	exec("ALTER TABLE group_payments ADD CONSTRAINT edit_failure CHECK (amount_rub <= 3000) NOT VALID")
	step("ledger_edit_save", "", "Не удалось сохранить изменение.")
	checkSaved()
	exec("ALTER TABLE group_payments DROP CONSTRAINT edit_failure")
	step("ledger_edit_save", "", "")
	wantGroup = "(1,1,1,3500,2026-11-15,2027-01-01,group-own)"
	checkSaved()
	step("ledger_edit_save", "", "Оплаты:")
	checkSaved()
	groupAmount := strings.Replace(group, "3000", "3500", 1)
	groupMonth := strings.Replace(groupAmount, "01.2027", "02.2027", 1)
	personalAmount := strings.Replace(personal, "1000", "1500", 1)
	for _, tc := range []struct{ index, field, input, invalid, detail, group, personal string }{
		{"0", "month", "02.2027", "13.2027", groupAmount, "(1,1,1,3500,2026-11-15,2027-02-01,group-own)", ""},
		{"0", "received", "16.11.2026", "31.02.2026", groupMonth, "(1,1,1,3500,2026-11-16,2027-02-01,group-own)", ""},
		{"1", "amount", "1500", "", personal, "", "(1,1,,1500,2026-11-14,,own)"},
		{"1", "received", "17.11.2026", "", personalAmount, "", "(1,1,,1500,2026-11-17,,own)"},
	} {
		step("ledger_view:"+tc.index, "", tc.detail)
		step("ledger_edit:"+tc.index, "", "оплат")
		step("ledger_edit_"+tc.field, "", "Введите ")
		if tc.invalid != "" {
			step("", tc.invalid, "Некорректная дата.")
		}
		step("", tc.input, "Проверьте изменение:")
		step("ledger_edit_save", "", "Оплаты:")
		if tc.group != "" {
			wantGroup = tc.group
		} else {
			wantPersonal = tc.personal
		}
		checkSaved()
	}
	step("ledger_view:0", "", strings.Replace(personalAmount, "14.11.2026", "17.11.2026", 1))
	step("ledger_page:0", "", "Оплаты:")
	step("ledger_view:1", "", strings.Replace(groupMonth, "15.11.2026", "16.11.2026", 1))
}
