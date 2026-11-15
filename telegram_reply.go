package main

import (
	"context"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func (h *telegramHandler) reply(ctx context.Context, b *bot.Bot, chatID int64, messageID int, text string, keyboard models.InlineKeyboardMarkup) (int, error) {
	if messageID != 0 {
		_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{ChatID: chatID, MessageID: messageID, Text: text, ReplyMarkup: keyboard})
		return messageID, err
	}
	sent, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: text, ReplyMarkup: keyboard})
	if err != nil {
		return 0, err
	}
	return sent.ID, nil
}
