package main

import (
	"context"
	"errors"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/jackc/pgx/v5"
)

const clientRenamePrompt = "Введите новое имя клиента."
const clientEditDatePrompt = "Введите дату рождения (ДД.ММ.ГГГГ)."

func (h *telegramHandler) handleClientEdit(ctx context.Context, b *bot.Bot, chatID, id int64, action, input string) {
	key := strconv.FormatInt(id, 10)
	back := "client_view:" + key
	if h.search != nil {
		back = "search_view:" + key
	}
	waiting := action != "edit_birth" && action != "edit_clear"
	birth := action == "edit_birth" || action == "edit_date" || action == "edit_clear" || (action == "input" && h.editBirthDate)
	text := clientRenamePrompt
	if birth {
		text = clientEditDatePrompt
		if waiting {
			back = "edit_birth:" + key
		}
	}
	keyboard := models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "⬅️ Назад", CallbackData: back}},
	}}
	if !waiting {
		text = "Изменение даты рождения."
		keyboard.InlineKeyboard = append([][]models.InlineKeyboardButton{
			{{Text: "📅 Ввести новую дату", CallbackData: "edit_date:" + key}},
			{{Text: "🗑️ Удалить дату", CallbackData: "edit_clear:" + key}},
		}, keyboard.InlineKeyboard...)
	}
	save := action == "edit_clear"
	var value any
	if action == "input" {
		input = strings.TrimSpace(input)
		value, save = input, input != ""
		if birth {
			date, err := time.Parse("02.01.2006", input)
			save = err == nil && date.Year() >= 1
			value = date
			if !save {
				text = "Некорректная дата.\n" + clientEditDatePrompt
			}
		} else if !save {
			text = "Имя не должно быть пустым.\n" + clientRenamePrompt
		}
	}
	if save {
		err := updateClientField(ctx, h.pool, id, birth, value)
		if err == nil || errors.Is(err, pgx.ErrNoRows) {
			waiting = false
			text, keyboard = h.clientCard(ctx, id)
			if h.search != nil {
				keyboard.InlineKeyboard[len(keyboard.InlineKeyboard)-1][0].CallbackData = "search_results"
			}
		} else {
			log.Printf("Update client failed: %v", err)
			text = "Не удалось сохранить изменения. Попробуйте ещё раз.\n" + text
		}
	}
	if _, err := h.reply(ctx, b, chatID, text, keyboard); err != nil {
		log.Printf("Telegram reply failed: %v", err)
		return
	}
	h.editClientID = 0
	if waiting {
		h.editClientID, h.editBirthDate = id, birth
	}
}
