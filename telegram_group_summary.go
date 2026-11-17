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

const groupSummaryMonthPrompt = "Введите месяц занятий (ММ.ГГГГ)."

type groupSummary struct {
	groupID      int64
	name         string
	waitingMonth bool
}

func (h *telegramHandler) handleGroupSummary(ctx context.Context, b *bot.Bot, chatID, groupID int64, action, input string) {
	next := groupSummary{groupID: groupID}
	if action != "summary" {
		if h.groupSummary == nil {
			return
		}
		next = *h.groupSummary
	}
	var err error
	if action == "summary" {
		next.name, err = getGroupName(ctx, h.pool, groupID)
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
	text := title + "\nВыберите период."
	keyboard := models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "📅 Выбрать месяц", CallbackData: "summary_month"}},
		{{Text: "🗓️ За всё время", CallbackData: "summary_all"}},
		{{Text: "⬅️ Назад", CallbackData: "group:" + strconv.FormatInt(next.groupID, 10) + "|" + strconv.FormatInt(h.revision+1, 10)}},
	}}
	if next.waitingMonth || action == "summary_all" || month != nil {
		text = problem + groupSummaryMonthPrompt
		keyboard.InlineKeyboard = [][]models.InlineKeyboardButton{{{Text: "⬅️ Назад", CallbackData: "summary_back"}}}
	}
	if action == "summary_all" || month != nil {
		var total string
		total, err = groupPaymentTotal(ctx, h.pool, next.groupID, month)
		period := "За всё время"
		if month != nil {
			period = month.Format("01.2006")
		}
		text = fmt.Sprintf("%s\nПериод: %s\nИтого: %s ₽", title, period, total)
	}
	if err != nil {
		log.Printf("Load group summary failed: %v", err)
		text = "Не удалось загрузить сводку. Попробуйте ещё раз."
		if action == "summary" {
			keyboard.InlineKeyboard = keyboard.InlineKeyboard[2:]
		}
	}
	if _, err := h.reply(ctx, b, chatID, text, keyboard); err != nil {
		log.Printf("Telegram group summary reply failed: %v", err)
	} else {
		h.groupSummary = &next
	}
}
