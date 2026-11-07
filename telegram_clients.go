package main

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const clientNamePrompt = "Введите имя клиента. Для отмены — /start."
const clientBirthDatePrompt = "Введите дату рождения (ДД.ММ.ГГГГ) или нажмите «Пропустить». Для отмены — /start."

func (h *telegramHandler) handleClientInput(ctx context.Context, b *bot.Bot, chatID int64, value string, skip bool) {
	keyboard := models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "Пропустить", CallbackData: "client_skip"}},
	}}
	if h.clientName == "" {
		name := strings.TrimSpace(value)
		text := clientBirthDatePrompt
		if name == "" {
			text, keyboard = "Имя не должно быть пустым.\n"+clientNamePrompt, h.menu
		}
		if sent, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: text, ReplyMarkup: keyboard}); err != nil {
			log.Printf("Telegram sendMessage failed: %v", err)
		} else {
			h.clientName, h.clientPromptID = name, sent.ID
		}
		return
	}
	var birthDate *time.Time
	text := clientBirthDatePrompt
	if !skip {
		date, err := time.Parse("02.01.2006", strings.TrimSpace(value))
		if err != nil || date.Year() < 1 {
			text = "Некорректная дата.\n" + clientBirthDatePrompt
		} else {
			birthDate = &date
		}
	}
	if skip || birthDate != nil {
		if err := createClient(ctx, h.pool, h.clientName, birthDate, h.clientGroupID); err != nil {
			log.Printf("Create client failed: %v", err)
			text = "Не удалось сохранить клиента.\n" + clientBirthDatePrompt
		} else {
			h.clientGroupID, h.clientName, h.clientPromptID = 0, "", 0
			text, keyboard = "Клиент создан.", h.menu
		}
	}
	if sent, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: text, ReplyMarkup: keyboard}); err != nil {
		log.Printf("Telegram sendMessage failed: %v", err)
	} else if h.clientGroupID > 0 {
		h.clientPromptID = sent.ID
	}
}
