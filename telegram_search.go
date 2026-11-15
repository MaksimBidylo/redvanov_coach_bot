package main

import (
	"context"
	"log"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const clientSearchPrompt = "Введите имя или его часть."

type clientSearch struct {
	query     string
	messageID int
}

func (h *telegramHandler) handleClientSearch(ctx context.Context, b *bot.Bot, chatID int64, query string) {
	query = strings.TrimSpace(query)
	text, keyboard := clientSearchPrompt, models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "⬅️ Назад", CallbackData: "menu"}},
	}}
	ready := false
	if query != "" {
		clients, err := searchClients(ctx, h.pool, query)
		switch {
		case err != nil:
			log.Printf("Search clients failed: %v", err)
			text = "Не удалось найти клиентов. Попробуйте ещё раз.\n" + clientSearchPrompt
		case len(clients) == 0:
			text = "Клиенты не найдены.\n" + clientSearchPrompt
		case len(clients) > 20:
			text = "Найдено больше 20 клиентов. Уточните запрос.\n" + clientSearchPrompt
		default:
			text, ready = "Результаты поиска:", true
			buttons := make([][]models.InlineKeyboardButton, 0, len(clients)+1)
			for _, c := range clients {
				label, groupName := c.Name, "без группы"
				if c.Archived {
					label += " (арх)"
				}
				if c.BirthDate != nil {
					label += " (" + c.BirthDate.Format("02.01.2006") + ")"
				}
				if c.GroupName != nil {
					groupName = *c.GroupName
				}
				buttons = append(buttons, []models.InlineKeyboardButton{{Text: label + " — " + groupName, CallbackData: "search_view:" + strconv.FormatInt(c.ID, 10)}})
			}
			keyboard = models.InlineKeyboardMarkup{InlineKeyboard: append(buttons, []models.InlineKeyboardButton{{Text: "⬅️ Назад", CallbackData: "menu"}})}
		}
	}
	messageID, err := h.reply(ctx, b, chatID, text, keyboard)
	if err != nil {
		log.Printf("Telegram search reply failed: %v", err)
		return
	}
	h.search = &clientSearch{}
	if ready {
		h.search.query, h.search.messageID = query, messageID
	}
}
