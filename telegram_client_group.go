package main

import (
	"context"
	"log"
	"strconv"

	"github.com/go-telegram/bot/models"
)

func (h *telegramHandler) changeClientGroup(ctx context.Context, clientID, groupID int64, save bool) (string, models.InlineKeyboardMarkup) {
	if save {
		err := setClientGroup(ctx, h.pool, clientID, groupID)
		text, keyboard := h.clientCard(ctx, clientID)
		if err != nil {
			log.Printf("Change client group failed: %v", err)
			text = "Не удалось изменить группу. Попробуйте ещё раз.\n\n" + text
		}
		return text, keyboard
	}
	id := strconv.FormatInt(clientID, 10)
	back := "client_view:" + id
	if h.search != nil {
		back = "search_view:" + id
	}
	keyboard := models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "Назад", CallbackData: back}},
	}}
	groups, err := listGroups(ctx, h.pool)
	if err != nil {
		log.Printf("Load groups failed: %v", err)
		return "Не удалось загрузить группы. Попробуйте ещё раз.", keyboard
	}
	buttons := make([][]models.InlineKeyboardButton, 0, len(groups)+2)
	for _, group := range groups {
		buttons = append(buttons, []models.InlineKeyboardButton{{Text: group.Name, CallbackData: "move:" + id + ":" + strconv.FormatInt(group.ID, 10)}})
	}
	buttons = append(buttons, []models.InlineKeyboardButton{{Text: "Без группы", CallbackData: "move:" + id + ":0"}})
	keyboard.InlineKeyboard = append(buttons, keyboard.InlineKeyboard...)
	return "Выберите группу:", keyboard
}
