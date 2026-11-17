package main

import (
	"context"
	"errors"
	"log"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const groupRenamePrompt = "Введите новое название группы."

func (h *telegramHandler) groupCard(ctx context.Context, id int64, name string) (string, models.InlineKeyboardMarkup) {
	body, buttons := h.groupClientsMenu(ctx, &id)
	key := strconv.FormatInt(id, 10)
	buttons = append(buttons,
		[]models.InlineKeyboardButton{{Text: "📊 Сводка", CallbackData: "summary:" + key}},
		[]models.InlineKeyboardButton{{Text: "✏️ Изменить название", CallbackData: "rename:" + key}},
		[]models.InlineKeyboardButton{{Text: "➕ Добавить клиента", CallbackData: "client:" + key}},
		[]models.InlineKeyboardButton{{Text: "📦 В архив", CallbackData: "gar:" + key}},
		[]models.InlineKeyboardButton{{Text: "🗑️ Удалить группу", CallbackData: "del:" + key}},
		[]models.InlineKeyboardButton{{Text: "⬅️ Назад", CallbackData: "groups"}},
	)
	return name + "\n\n" + body, models.InlineKeyboardMarkup{InlineKeyboard: buttons}
}

func (h *telegramHandler) handleGroupRename(ctx context.Context, b *bot.Bot, chatID int64, value string) {
	id := h.renameGroupID
	name := strings.TrimSpace(value)
	text := "Название не должно быть пустым.\n" + groupRenamePrompt
	keyboard := models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "⬅️ Назад", CallbackData: "group:" + strconv.FormatInt(id, 10)}},
	}}
	if name != "" {
		err := renameGroup(ctx, h.pool, id, name)
		var pgError *pgconn.PgError
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			h.renameGroupID = 0
			text = "Группа не найдена."
			keyboard.InlineKeyboard[0][0].CallbackData = "groups"
		case errors.As(err, &pgError) && pgError.Code == "23505":
			text = "Группа с таким названием уже есть.\n" + groupRenamePrompt
		case err != nil:
			log.Printf("Rename group failed: %v", err)
			text = "Не удалось изменить название группы. Попробуйте ещё раз.\n" + groupRenamePrompt
		default:
			h.renameGroupID = 0
			text, keyboard = h.groupCard(ctx, id, name)
		}
	}
	if _, err := h.reply(ctx, b, chatID, text, keyboard); err != nil {
		h.renameGroupID = id
		log.Printf("Telegram reply failed: %v", err)
	}
}
