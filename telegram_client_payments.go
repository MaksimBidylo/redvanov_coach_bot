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

func (h *telegramHandler) handleClientPayments(ctx context.Context, b *bot.Bot, chatID int64, action string, id int64) {
	history := h.paymentHistory
	if action == "ledger" {
		history = &clientPaymentHistory{clientID: id}
	}
	if history == nil {
		return
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
	case "ledger_view":
		if id >= int64(len(history.payments)) {
			return
		}
		payment := history.payments[id]
		text = fmt.Sprintf("Персональная оплата\nСумма: %d ₽\nДата получения денег: %s", payment.AmountRub, payment.ReceivedOn.Format("02.01.2006"))
		if !payment.Personal {
			text = fmt.Sprintf("Групповая оплата\nСумма: %d ₽\nДата получения денег: %s\nГруппа: %s\nМесяц занятий: %s", payment.AmountRub, payment.ReceivedOn.Format("02.01.2006"), *payment.GroupName, payment.TrainingMonth.Format("01.2006"))
		}
		keyboard.InlineKeyboard[0][0].CallbackData = "ledger_page:" + strconv.FormatInt(history.page, 10)
	case "ledger", "ledger_page":
		if action == "ledger_page" {
			next.page = id
		}
		if next.page > (1<<63-1)/10 {
			return
		}
		payments, err := listClientPayments(ctx, h.pool, next.clientID, next.page)
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
	if _, err := h.reply(ctx, b, chatID, text, keyboard); err != nil {
		log.Printf("Telegram payment history reply failed: %v", err)
	} else {
		h.paymentHistory = &next
	}
}
