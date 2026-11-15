package main

import (
	"context"
	"errors"
	"log"
	"strconv"

	"github.com/go-telegram/bot/models"
	"github.com/jackc/pgx/v5"
)

func (h *telegramHandler) closeGroupMenu(ctx context.Context, id, target int64, selected, confirm, archive bool) (string, models.InlineKeyboardMarkup) {
	prefix, verb, title := "del", "удалить", "Удалить"
	icon := "🗑️"
	before, prepare, result := "удалением", "удаление", "Группа удалена."
	if archive {
		icon = "📦"
		prefix, verb, title = "gar", "архивировать", "Архивировать"
		before, prepare, result = "архивацией", "архивацию", "Группа отправлена в архив."
	}
	key := strconv.FormatInt(id, 10)
	keyboard := models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "⬅️ Назад", CallbackData: "groups"}},
	}}
	name, err := getGroupName(ctx, h.pool, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "Группа не найдена.", keyboard
	}
	if err != nil {
		log.Printf("Load group failed: %v", err)
		return "Не удалось подготовить " + prepare + " группы. Попробуйте ещё раз.", keyboard
	}
	keyboard.InlineKeyboard[0][0].CallbackData = "group:" + key
	if confirm {
		if err := closeGroup(ctx, h.pool, id, target, archive); err != nil {
			log.Printf("Close group failed: %v", err)
			return "Не удалось " + verb + " группу. Попробуйте ещё раз.", keyboard
		}
		keyboard.InlineKeyboard[0][0].CallbackData = "groups"
		return result, keyboard
	}
	clients, err := listGroupClients(ctx, h.pool, &id, true)
	if err != nil {
		log.Printf("Load group clients failed: %v", err)
		return "Не удалось подготовить " + prepare + " группы. Попробуйте ещё раз.", keyboard
	}
	if len(clients) == 0 {
		selected, target = true, 0
	}
	if !selected {
		groups, err := listGroups(ctx, h.pool, false)
		if err != nil {
			log.Printf("Load groups failed: %v", err)
			return "Не удалось загрузить группы. Попробуйте ещё раз.", keyboard
		}
		buttons := make([][]models.InlineKeyboardButton, 0, len(groups)+1)
		for _, group := range groups {
			if group.ID != id {
				buttons = append(buttons, []models.InlineKeyboardButton{{Text: group.Name, CallbackData: prefix + ":" + key + ":" + strconv.FormatInt(group.ID, 10)}})
			}
		}
		buttons = append(buttons, []models.InlineKeyboardButton{{Text: "👤 Без группы", CallbackData: prefix + ":" + key + ":0"}})
		keyboard.InlineKeyboard = append(buttons, keyboard.InlineKeyboard...)
		return "Куда перевести клиентов группы «" + name + "» перед " + before + "?", keyboard
	}
	destination := "Без группы"
	if target > 0 {
		destination, err = getGroupName(ctx, h.pool, target)
		if err != nil {
			log.Printf("Load target group failed: %v", err)
			return "Не удалось подготовить " + prepare + " группы. Попробуйте ещё раз.", keyboard
		}
	}
	text, button := title+" группу «"+name+"»?", title
	if len(clients) != 0 {
		text = "Перевести всех клиентов группы «" + name + "» в «" + destination + "» и " + verb + " группу?"
		button = "Перенести и " + verb
	}
	keyboard.InlineKeyboard = append([][]models.InlineKeyboardButton{
		{{Text: icon + " " + button, CallbackData: prefix + "c:" + key + ":" + strconv.FormatInt(target, 10)}},
	}, keyboard.InlineKeyboard...)
	return text, keyboard
}
