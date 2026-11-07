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
