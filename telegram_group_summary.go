package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const groupSummaryMonthPrompt = "Введите месяц занятий (ММ.ГГГГ)."

type paymentSummary struct {
	id           int64
	name         string
	personal     bool
	waitingMonth bool
	month        *time.Time
	exportReady  bool
}

func (h *telegramHandler) handleSummary(ctx context.Context, b *bot.Bot, chatID, id int64, action, input string) {
	entry := action == "summary" || action == "summary_personal"
	next := paymentSummary{id: id, personal: action == "summary_personal"}
	if !entry {
		if h.summary == nil {
			return
		}
		next = *h.summary
	}
	if action == "summary_export" {
		if next.personal || !next.exportReady {
			return
		}
		data, err := groupPaymentsExcel(ctx, h.pool, next.id, next.name, next.month)
		if err == nil {
			_, err = b.SendDocument(ctx, &bot.SendDocumentParams{ChatID: chatID, Document: &models.InputFileUpload{
				Filename: "group-payments.xlsx", Data: bytes.NewReader(data),
			}})
		}
		if err != nil {
			log.Printf("Export group payments failed: %v", err)
			next.exportReady = false
			keyboard := models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: "⬅️ Назад", CallbackData: "summary_back"}},
			}}
			if _, err := h.reply(ctx, b, chatID, "Не удалось выгрузить сводку. Попробуйте ещё раз.", keyboard); err != nil {
				log.Printf("Telegram export reply failed: %v", err)
			} else {
				h.summary = &next
			}
		}
		return
	}
	next.exportReady = false
	var err error
	if entry && id > 0 {
		if next.personal {
			var c client
			c, err = getClient(ctx, h.pool, id)
			next.name = c.Name
		} else {
			var g group
			g, err = getGroup(ctx, h.pool, id)
			next.name = g.Name
		}
	}
	wasWaiting := next.waitingMonth
	next.waitingMonth = action == "summary_month"
	var month *time.Time
	problem := ""
	if action == "input" {
		if !wasWaiting {
			return
		}
		date, parseErr := time.Parse("01.2006", strings.TrimSpace(input))
		if parseErr != nil || date.Year() < 1 {
			next.waitingMonth, problem = true, "Некорректная дата.\n"
		} else {
			month = &date
		}
	}
	title := "Сводка по группе «" + next.name + "»"
	prompt, back := groupSummaryMonthPrompt, "group:"+strconv.FormatInt(next.id, 10)
	if next.personal {
		title, prompt, back = "Персональные оплаты", "Введите месяц получения денег (ММ.ГГГГ).", "menu"
		if next.id > 0 {
			title += " клиента «" + next.name + "»"
			back = "client_view:" + strconv.FormatInt(next.id, 10)
			if h.search != nil && h.search.messageID != 0 {
				back = "search_view:" + strconv.FormatInt(next.id, 10)
			}
		}
	}
	if !strings.HasPrefix(back, "search_") {
		back += "|" + strconv.FormatInt(h.revision+1, 10)
	}
	text := title + "\nВыберите период."
	keyboard := models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "📅 Выбрать месяц", CallbackData: "summary_month"}},
		{{Text: "🗓️ За всё время", CallbackData: "summary_all"}},
		{{Text: "⬅️ Назад", CallbackData: back}},
	}}
	if next.waitingMonth || action == "summary_all" || month != nil {
		text = problem + prompt
		keyboard.InlineKeyboard = [][]models.InlineKeyboardButton{{{Text: "⬅️ Назад", CallbackData: "summary_back"}}}
	}
	if action == "summary_all" || month != nil {
		var total string
		if next.personal {
			total, err = personalPaymentTotal(ctx, h.pool, next.id, month)
		} else {
			total, err = groupPaymentTotal(ctx, h.pool, next.id, month)
		}
		period := "За всё время"
		if month != nil {
			period = month.Format("01.2006")
		}
		text = fmt.Sprintf("%s\nПериод: %s\nИтого: %s ₽", title, period, total)
		if err == nil && !next.personal {
			next.month, next.exportReady = month, true
			keyboard.InlineKeyboard = append([][]models.InlineKeyboardButton{{{Text: "📥 Excel", CallbackData: "summary_export"}}}, keyboard.InlineKeyboard...)
		}
	}
	if err != nil {
		log.Printf("Load payment summary failed: %v", err)
		text = "Не удалось загрузить сводку. Попробуйте ещё раз."
		if entry {
			keyboard.InlineKeyboard = keyboard.InlineKeyboard[2:]
		}
	}
	if _, err := h.reply(ctx, b, chatID, text, keyboard); err != nil {
		log.Printf("Telegram payment summary reply failed: %v", err)
	} else {
		h.summary = &next
	}
}
