package main

import (
	"context"
	"errors"
	"log"
	"strconv"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func newTelegramBot(owner, token string, options ...bot.Option) (*bot.Bot, error) {
	ownerID, err := strconv.ParseInt(owner, 10, 64)
	if err != nil || ownerID <= 0 {
		return nil, errors.New("TELEGRAM_OWNER_ID must be a positive integer")
	}
	if token == "" {
		return nil, errors.New("TELEGRAM_BOT_TOKEN is required")
	}

	handler := func(ctx context.Context, b *bot.Bot, update *models.Update) {
		message := update.Message
		if message == nil || message.From == nil {
			return
		}
		if message.From.ID != ownerID || message.Chat.Type != models.ChatTypePrivate || message.Text != "/start" {
			return
		}
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: message.Chat.ID,
			Text:   "Добро пожаловать!",
		})
		if err != nil {
			log.Printf("Telegram sendMessage failed: %v", err)
		}
	}

	opts := []bot.Option{
		bot.WithDefaultHandler(handler),
		bot.WithSkipGetMe(),
	}
	opts = append(opts, options...)

	b, err := bot.New(token, opts...)

	if err != nil {
		return nil, err
	}

	return b, nil
}
