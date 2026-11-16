package main

import (
	"context"
	"fmt"
	"log"
	"strconv"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type clientPaymentHistory struct {
	clientID, page int64
	payments       []clientPayment
}

func (p clientPayment) details() string {
	text := fmt.Sprintf("Персональная оплата\nСумма: %d ₽\nДата получения денег: %s", p.AmountRub, p.ReceivedOn.Format("02.01.2006"))
	if !p.Personal {
		text = fmt.Sprintf("Групповая оплата\nСумма: %d ₽\nДата получения денег: %s\nГруппа: %s\nМесяц занятий: %s", p.AmountRub, p.ReceivedOn.Format("02.01.2006"), *p.GroupName, p.TrainingMonth.Format("01.2006"))
	}
	return text
}

func (h *telegramHandler) handleClientPayments(ctx context.Context, b *bot.Bot, chatID int64, action string, id int64) {
	history := h.paymentHistory
	if action == "ledger" {
		history = &clientPaymentHistory{clientID: id}
	}
	if history == nil {
		return
	}
	problem := ""
	if action == "ledger_confirm" {
		if id >= int64(len(history.payments)) {
			return
		}
		payment := history.payments[id]
		if err := deleteClientPayment(ctx, h.pool, history.clientID, payment.ID, payment.Personal); err != nil {
			log.Printf("Delete client payment failed: %v", err)
			problem, action = "Не удалось удалить оплату. Попробуйте ещё раз.\n\n", "ledger_delete"
		} else {
			problem, action, id = "Оплата удалена.\n\n", "ledger_page", history.page
		}
	}
	next := *history
	text := "Оплаты:"
	keyboard := models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "⬅️ Назад", CallbackData: "ledger_client"}},
	}}
	switch action {
	case "ledger_client":
		text, keyboard = h.clientCard(ctx, history.clientID)
		if h.search != nil {
			keyboard.InlineKeyboard[len(keyboard.InlineKeyboard)-1][0].CallbackData = "search_results"
		}
	case "ledger_view", "ledger_delete":
		if id >= int64(len(history.payments)) {
			return
		}
		payment := history.payments[id]
		text = payment.details()
		keyboard.InlineKeyboard[0][0].CallbackData = "ledger_page:" + strconv.FormatInt(history.page, 10)
		button, callback := "🗑️ Удалить", "ledger_delete:"
		if action == "ledger_delete" {
			text = "Удалить оплату?\n\n" + text
			button, callback = "🗑️ Подтвердить удаление", "ledger_confirm:"
			keyboard.InlineKeyboard[0][0].CallbackData = "ledger_view:" + strconv.FormatInt(id, 10)
		}
		keyboard.InlineKeyboard = append([][]models.InlineKeyboardButton{{{Text: button, CallbackData: callback + strconv.FormatInt(id, 10)}}}, keyboard.InlineKeyboard...)
		if action == "ledger_view" {
			keyboard.InlineKeyboard = append([][]models.InlineKeyboardButton{{{Text: "✏️ Изменить", CallbackData: "ledger_edit:" + strconv.FormatInt(id, 10)}}}, keyboard.InlineKeyboard...)
		}
	case "ledger", "ledger_page":
		if action == "ledger_page" {
			next.page = id
		}
		if next.page > (1<<63-1)/10 {
			return
		}
		payments, err := listClientPayments(ctx, h.pool, next.clientID, next.page)
		if err == nil && len(payments) == 0 && next.page > 0 {
			next.page--
			payments, err = listClientPayments(ctx, h.pool, next.clientID, next.page)
		}
		next.payments = nil
		switch {
		case err != nil:
			log.Printf("Load client payments failed: %v", err)
			text = "Не удалось загрузить оплаты. Попробуйте ещё раз."
		case len(payments) == 0:
			text = "Оплат пока нет."
		default:
			next.payments = payments[:min(10, len(payments))]
			var buttons [][]models.InlineKeyboardButton
			for i, payment := range next.payments {
				kind := "групповая"
				if payment.Personal {
					kind = "персональная"
				}
				buttons = append(buttons, []models.InlineKeyboardButton{{Text: fmt.Sprintf("%s · %d ₽ · %s", payment.ReceivedOn.Format("02.01.2006"), payment.AmountRub, kind), CallbackData: "ledger_view:" + strconv.Itoa(i)}})
			}
			if next.page > 0 {
				buttons = append(buttons, []models.InlineKeyboardButton{{Text: "⬅️ Предыдущие", CallbackData: "ledger_page:" + strconv.FormatInt(next.page-1, 10)}})
			}
			if len(payments) > 10 {
				buttons = append(buttons, []models.InlineKeyboardButton{{Text: "➡️ Следующие", CallbackData: "ledger_page:" + strconv.FormatInt(next.page+1, 10)}})
			}
			keyboard.InlineKeyboard = append(buttons, keyboard.InlineKeyboard...)
		}
	}
	if _, err := h.reply(ctx, b, chatID, problem+text, keyboard); err != nil {
		log.Printf("Telegram payment history reply failed: %v", err)
	} else {
		h.paymentHistory = &next
		h.paymentEdit = nil
	}
}
