package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type paymentEdit struct {
	index     int64
	field     string
	value     any
	attempted bool
}

func (h *telegramHandler) handlePaymentEdit(ctx context.Context, b *bot.Bot, chatID, index int64, action, input string) {
	draft := paymentEdit{index: index}
	if h.paymentEdit != nil {
		draft = *h.paymentEdit
	} else if action != "ledger_edit" {
		return
	}
	history := h.paymentHistory
	if history == nil || draft.index >= int64(len(history.payments)) {
		return
	}
	payment := history.payments[draft.index]
	fields := map[string]string{"ledger_edit_amount": "Сумма", "ledger_edit_received": "Дата получения денег", "ledger_edit_month": "Месяц занятий"}
	if fields[action] != "" {
		if draft.attempted || (payment.Personal && action == "ledger_edit_month") {
			return
		}
		draft.field, draft.value = action, nil
	}
	if action == "ledger_edit_back" {
		action, index = "ledger_view", draft.index
		if draft.attempted {
			action, index = "ledger_page", history.page
		}
		h.handleClientPayments(ctx, b, chatID, action, index)
		return
	}
	problem := ""
	if action == "ledger_edit_save" {
		if draft.value == nil {
			return
		}
		draft.attempted, h.paymentEdit = true, &draft
		if err := updateClientPayment(ctx, h.pool, history.clientID, payment.ID, payment.Personal, draft.field, draft.value); err != nil {
			log.Printf("Update payment failed: %v", err)
			problem = "Не удалось сохранить изменение. Попробуйте ещё раз.\n\n"
		} else {
			h.handleClientPayments(ctx, b, chatID, "ledger_page", history.page)
			return
		}
	}
	text, layout := "Введите сумму оплаты.", "02.01.2006"
	if draft.field == "ledger_edit_received" {
		text = "Введите дату получения денег (ДД.ММ.ГГГГ)."
	} else if draft.field == "ledger_edit_month" {
		text, layout = "Введите месяц занятий (ММ.ГГГГ).", "01.2006"
	}
	if action == "input" {
		if draft.field == "" || draft.value != nil {
			return
		}
		input = strings.TrimSpace(input)
		if draft.field == "ledger_edit_amount" {
			amount, err := strconv.ParseInt(input, 10, 64)
			if err == nil && amount > 0 {
				draft.value = amount
			} else {
				problem = "Введите положительную целую сумму.\n"
			}
		} else {
			date, err := time.Parse(layout, input)
			if err == nil && date.Year() >= 1 {
				draft.value = date
			} else {
				problem = "Некорректная дата.\n"
			}
		}
	}
	keyboard := models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "⬅️ Назад", CallbackData: "ledger_edit_back"}},
	}}
	if draft.field == "" {
		text = "Выберите поле для изменения.\n\n" + payment.details()
		buttons := [][]models.InlineKeyboardButton{
			{{Text: "💰 Сумма", CallbackData: "ledger_edit_amount"}},
			{{Text: "📅 Дата получения денег", CallbackData: "ledger_edit_received"}},
		}
		if !payment.Personal {
			buttons = append(buttons, []models.InlineKeyboardButton{{Text: "📅 Месяц занятий", CallbackData: "ledger_edit_month"}})
		}
		keyboard.InlineKeyboard = append(buttons, keyboard.InlineKeyboard...)
	} else if draft.value != nil {
		value := fmt.Sprint(draft.value)
		if date, ok := draft.value.(time.Time); ok {
			value = date.Format(layout)
		}
		text = "Проверьте изменение:\n\n" + payment.details() + "\n\n" + fields[draft.field] + " → " + value
		keyboard.InlineKeyboard = append([][]models.InlineKeyboardButton{{{Text: "💾 Сохранить", CallbackData: "ledger_edit_save"}}}, keyboard.InlineKeyboard...)
	}
	if _, err := h.reply(ctx, b, chatID, problem+text, keyboard); err != nil {
		log.Printf("Telegram payment edit reply failed: %v", err)
	} else {
		h.paymentEdit = &draft
	}
}
