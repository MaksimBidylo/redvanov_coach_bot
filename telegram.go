package main

import (
	"context"
	"errors"
	"log"
	"strconv"
	"strings"

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
	handler := func(ctx context.Context, b *bot.Bot, update *models.Update) {
		if query := update.CallbackQuery; query != nil {
			message := query.Message.Message
			if query.From.ID != h.ownerID || message == nil || message.Chat.Type != models.ChatTypePrivate {
				return
			}
			if query.Data != "groups" && query.Data != "menu" && query.Data != "group_create" {
				return
			}
			if query.Data != "group_create" {
				h.waitingForGroupName.Store(false)
			}
			if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID}); err != nil {
				log.Printf("Telegram answerCallbackQuery failed: %v", err)
			}
			if query.Data == "group_create" {
				if _, err := b.SendMessage(ctx, &bot.SendMessageParams{
					ChatID: message.Chat.ID, Text: groupNamePrompt, ReplyMarkup: h.menu,
				}); err != nil {
					log.Printf("Telegram sendMessage failed: %v", err)
				} else {
					h.waitingForGroupName.Store(true)
				}
				return
			}
			text, keyboard := "Добро пожаловать!", h.menu
			if query.Data == "groups" {
				keyboard = models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
					{{Text: "Добавить группу", CallbackData: "group_create"}},
					{{Text: "Назад", CallbackData: "menu"}},
				}}
				names, err := listGroupNames(ctx, h.pool)
				switch {
				case err != nil:
					log.Printf("Load groups failed: %v", err)
					text = "Не удалось загрузить группы. Попробуйте ещё раз."
				case len(names) == 0:
					text = "Групп пока нет"
				default:
					text = "Группы:\n" + strings.Join(names, "\n")
				}
			}
			if _, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
				ChatID: message.Chat.ID, MessageID: message.ID, Text: text, ReplyMarkup: keyboard,
			}); err != nil {
				log.Printf("Telegram editMessageText failed: %v", err)
			}
			return
		}
		h.handleMessage(ctx, b, update.Message)
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
