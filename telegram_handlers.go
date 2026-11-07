package main

import (
	"context"
	"log"
	"strings"
	"sync/atomic"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

const groupNamePrompt = "Введите название группы. Для отмены — /start."

type telegramHandler struct {
	ownerID             int64
	pool                *pgxpool.Pool
	menu                models.InlineKeyboardMarkup
	waitingForGroupName atomic.Bool
}

func (h *telegramHandler) handle(ctx context.Context, b *bot.Bot, update *models.Update) {
	if query := update.CallbackQuery; query != nil {
		h.handleCallback(ctx, b, query)
		return
	}
	h.handleMessage(ctx, b, update.Message)
}

func (h *telegramHandler) handleCallback(ctx context.Context, b *bot.Bot, query *models.CallbackQuery) {
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
}

func (h *telegramHandler) handleMessage(ctx context.Context, b *bot.Bot, message *models.Message) {
	if message == nil || message.From == nil {
		return
	}
	if message.From.ID != h.ownerID || message.Chat.Type != models.ChatTypePrivate {
		return
	}
	text := "Добро пожаловать!"
	if message.Text == "/start" {
		h.waitingForGroupName.Store(false)
	} else {
		if !h.waitingForGroupName.Load() {
			return
		}
		name := strings.TrimSpace(message.Text)
		text = "Название не должно быть пустым.\n" + groupNamePrompt
		if name != "" {
			created, err := createGroup(ctx, h.pool, name)
			switch {
			case err != nil:
				log.Printf("Create group failed: %v", err)
				text = "Не удалось создать группу. Попробуйте ещё раз.\n" + groupNamePrompt
			case !created:
				text = "Группа с таким названием уже есть.\n" + groupNamePrompt
			default:
				h.waitingForGroupName.Store(false)
				text = "Группа создана."
			}
		}
	}
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: message.Chat.ID, Text: text, ReplyMarkup: h.menu,
	})
	if err != nil {
		log.Printf("Telegram sendMessage failed: %v", err)
	}
}
