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

	menu := models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "Группы", CallbackData: "groups"}},
	}}
	handler := func(ctx context.Context, b *bot.Bot, update *models.Update) {
		if query := update.CallbackQuery; query != nil {
			message := query.Message.Message
			if query.From.ID != ownerID || message == nil || message.Chat.Type != models.ChatTypePrivate {
				return
			}
			if query.Data != "groups" && query.Data != "menu" {
				return
			}
			if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID}); err != nil {
				log.Printf("Telegram answerCallbackQuery failed: %v", err)
			}
			text, keyboard := "Добро пожаловать!", menu
			if query.Data == "groups" {
				keyboard = models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
					{{Text: "Назад", CallbackData: "menu"}},
				}}
				names, err := listGroupNames(ctx, pool)
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
		message := update.Message
		if message == nil || message.From == nil {
			return
		}
		if message.From.ID != ownerID || message.Chat.Type != models.ChatTypePrivate || message.Text != "/start" {
			return
		}
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: message.Chat.ID, Text: "Добро пожаловать!", ReplyMarkup: menu,
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
