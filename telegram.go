package main

import (
	"errors"
	"strconv"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newTelegramBot(owner, token string, pool *pgxpool.Pool, options ...bot.Option) (*bot.Bot, error) {
	ownerID, err := strconv.ParseInt(owner, 10, 64)
	if err != nil || ownerID <= 0 {
		return nil, errors.New("TELEGRAM_OWNER_ID must be a positive integer")
	}
	if token == "" {
		return nil, errors.New("TELEGRAM_BOT_TOKEN is required")
	}

	h := &telegramHandler{
		ownerID: ownerID,
		pool:    pool,
		menu: models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "Группы", CallbackData: "groups"}},
		}},
	}

	opts := []bot.Option{
		bot.WithDefaultHandler(h.handle),
		bot.WithSkipGetMe(),
		bot.WithNotAsyncHandlers(),
	}
	opts = append(opts, options...)

	b, err := bot.New(token, opts...)

	if err != nil {
		return nil, err
	}

	return b, nil
}
