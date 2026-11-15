package main

import (
	"context"
	"errors"
	"log"
	"strconv"

	"github.com/go-telegram/bot/models"
	"github.com/jackc/pgx/v5"
)

func (h *telegramHandler) deleteGroupMenu(ctx context.Context, id, target int64, selected, confirm bool) (string, models.InlineKeyboardMarkup) {
	key := strconv.FormatInt(id, 10)
	keyboard := models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "Назад", CallbackData: "groups"}},
	}}
	name, err := getGroupName(ctx, h.pool, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "Группа не найдена.", keyboard
	}
	if err != nil {
		log.Printf("Load group failed: %v", err)
		return "Не удалось подготовить удаление группы. Попробуйте ещё раз.", keyboard
	}
	keyboard.InlineKeyboard[0][0].CallbackData = "group:" + key
	if confirm {
		if err := deleteGroup(ctx, h.pool, id, target); err != nil {
			log.Printf("Delete group failed: %v", err)
			return "Не удалось удалить группу. Попробуйте ещё раз.", keyboard
		}
		keyboard.InlineKeyboard[0][0].CallbackData = "groups"
		return "Группа удалена.", keyboard
	}
	clients, err := listGroupClients(ctx, h.pool, &id)
	if err != nil {
		log.Printf("Load group clients failed: %v", err)
		return "Не удалось подготовить удаление группы. Попробуйте ещё раз.", keyboard
	}
	if len(clients) == 0 {
		selected, target = true, 0
	}
	if !selected {
		groups, err := listGroups(ctx, h.pool)
		if err != nil {
			log.Printf("Load groups failed: %v", err)
			return "Не удалось загрузить группы. Попробуйте ещё раз.", keyboard
		}
		buttons := make([][]models.InlineKeyboardButton, 0, len(groups)+1)
		for _, group := range groups {
			if group.ID != id {
				buttons = append(buttons, []models.InlineKeyboardButton{{Text: group.Name, CallbackData: "del:" + key + ":" + strconv.FormatInt(group.ID, 10)}})
			}
		}
		buttons = append(buttons, []models.InlineKeyboardButton{{Text: "Без группы", CallbackData: "del:" + key + ":0"}})
		keyboard.InlineKeyboard = append(buttons, keyboard.InlineKeyboard...)
		return "Куда перевести клиентов группы «" + name + "» перед удалением?", keyboard
	}
	destination := "Без группы"
	if target > 0 {
		destination, err = getGroupName(ctx, h.pool, target)
		if err != nil {
			log.Printf("Load target group failed: %v", err)
			return "Не удалось подготовить удаление группы. Попробуйте ещё раз.", keyboard
		}
	}
	text, button := "Удалить группу «"+name+"»?", "Удалить"
	if len(clients) != 0 {
		text = "Перевести всех клиентов группы «" + name + "» в «" + destination + "» и удалить группу?"
		button = "Перенести и удалить"
	}
	keyboard.InlineKeyboard = append([][]models.InlineKeyboardButton{
		{{Text: button, CallbackData: "delc:" + key + ":" + strconv.FormatInt(target, 10)}},
	}, keyboard.InlineKeyboard...)
	return text, keyboard
}
