package main

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func (h *telegramHandler) reply(ctx context.Context, b *bot.Bot, chatID int64, text string, keyboard models.InlineKeyboardMarkup) (int, error) {
	messageID := h.messageID
	keyboard.InlineKeyboard = append([][]models.InlineKeyboardButton(nil), keyboard.InlineKeyboard...)
	for i, row := range keyboard.InlineKeyboard {
		keyboard.InlineKeyboard[i] = append([]models.InlineKeyboardButton(nil), row...)
		for j, button := range row {
			if button.CallbackData == "client_skip" || strings.HasPrefix(button.CallbackData, "search_") || strings.HasPrefix(button.CallbackData, "move:") || strings.HasPrefix(button.CallbackData, "rename:") || strings.HasPrefix(button.CallbackData, "edit_") || strings.HasPrefix(button.CallbackData, "del") {
				keyboard.InlineKeyboard[i][j].CallbackData += "|" + strconv.FormatInt(h.revision+1, 10)
			}
		}
	}
	var err error
	if messageID != 0 {
		_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{ChatID: chatID, MessageID: messageID, Text: text, ReplyMarkup: keyboard})
		if errors.Is(err, bot.ErrorBadRequest) && strings.Contains(err.Error(), "message is not modified") {
			err = nil
		}
		if errors.Is(err, bot.ErrorBadRequest) && strings.Contains(err.Error(), "message to edit not found") {
			messageID = 0
		}
	}
	if messageID == 0 {
		var sent *models.Message
		sent, err = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: text, ReplyMarkup: keyboard})
		if err == nil {
			messageID = sent.ID
		}
	}
	if err != nil {
		return 0, err
	}
	h.messageID, h.revision = messageID, h.revision+1
	return messageID, nil
}
