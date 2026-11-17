package main

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newTelegramBot(owners, token string, pool *pgxpool.Pool, options ...bot.Option) (*bot.Bot, error) {
	if token == "" {
		return nil, errors.New("TELEGRAM_BOT_TOKEN is required")
	}

	handlers := make(map[int64]*telegramHandler)
	for _, owner := range strings.Split(owners, ",") {
		ownerID, err := strconv.ParseInt(strings.TrimSpace(owner), 10, 64)
		if err != nil || ownerID <= 0 {
			return nil, errors.New("TELEGRAM_OWNER_ID must contain positive integers separated by commas")
		}
		handlers[ownerID] = &telegramHandler{
			ownerID:  ownerID,
			pool:     pool,
			revision: time.Now().UnixNano(),
			menu: models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: "👥 Группы", CallbackData: "groups"}},
				{{Text: "👤 Клиенты без группы", CallbackData: "ungrouped"}},
				{{Text: "🔍 Найти клиента", CallbackData: "search"}},
				{{Text: "📊 Персональные оплаты", CallbackData: "summary_personal"}},
			}},
		}
	}
	handler := func(ctx context.Context, b *bot.Bot, update *models.Update) {
		var userID int64
		if query := update.CallbackQuery; query != nil {
			userID = query.From.ID
		} else if message := update.Message; message != nil && message.From != nil {
			userID = message.From.ID
		}
		if h := handlers[userID]; h != nil {
			h.handle(ctx, b, update)
		}
	}

	opts := []bot.Option{
		bot.WithDefaultHandler(handler),
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
