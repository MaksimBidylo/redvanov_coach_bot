package main

import (
	"context"
	"errors"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/jackc/pgx/v5"
)

const clientNamePrompt = "Введите имя клиента. Для отмены — /start."
const clientBirthDatePrompt = "Введите дату рождения (ДД.ММ.ГГГГ) или нажмите «Пропустить». Для отмены — /start."

type clientDraft struct {
	groupID  *int64
	name     string
	promptID int
}

func (h *telegramHandler) groupClientsMenu(ctx context.Context, groupID *int64) (string, [][]models.InlineKeyboardButton) {
	clients, err := listGroupClients(ctx, h.pool, groupID, false)
	if err != nil {
		log.Printf("Load group clients failed: %v", err)
		return "Не удалось загрузить клиентов. Попробуйте ещё раз.", nil
	}
	if len(clients) == 0 {
		return "Клиентов пока нет.", nil
	}
	buttons := make([][]models.InlineKeyboardButton, len(clients))
	for i, client := range clients {
		label := client.Name
		if client.BirthDate != nil {
			label += " (" + client.BirthDate.Format("02.01.2006") + ")"
		}
		buttons[i] = []models.InlineKeyboardButton{{Text: label, CallbackData: "client_view:" + strconv.FormatInt(client.ID, 10)}}
	}
	return "Клиенты:", buttons
}

func (h *telegramHandler) clientCard(ctx context.Context, id int64) (string, models.InlineKeyboardMarkup) {
	keyboard := models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "⬅️ Назад", CallbackData: "groups"}},
	}}
	c, err := getClient(ctx, h.pool, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "Клиент не найден.", keyboard
	}
	if err != nil {
		log.Printf("Load client failed: %v", err)
		return "Не удалось загрузить клиента. Попробуйте ещё раз.", keyboard
	}
	if c.GroupID != nil {
		keyboard.InlineKeyboard[0][0].CallbackData = "group:" + strconv.FormatInt(*c.GroupID, 10)
	} else {
		keyboard.InlineKeyboard[0][0].CallbackData = "ungrouped"
	}
	birthDate, groupName := "не указана", "без группы"
	if c.BirthDate != nil {
		birthDate = c.BirthDate.Format("02.01.2006")
	}
	if c.GroupName != nil {
		groupName = *c.GroupName
	}
	name, archiveText, archiveAction := c.Name, "📦 В архив", "archive:"
	if c.Archived {
		name += " (арх)"
		archiveText, archiveAction = "♻️ Восстановить", "restore:"
	}
	keyboard.InlineKeyboard = append([][]models.InlineKeyboardButton{
		{{Text: "✏️ Изменить имя", CallbackData: "edit_name:" + strconv.FormatInt(c.ID, 10)}},
		{{Text: "📅 Изменить дату рождения", CallbackData: "edit_birth:" + strconv.FormatInt(c.ID, 10)}},
		{{Text: "🔄 Сменить группу", CallbackData: "move:" + strconv.FormatInt(c.ID, 10)}},
		{{Text: archiveText, CallbackData: archiveAction + strconv.FormatInt(c.ID, 10)}},
	}, keyboard.InlineKeyboard...)
	return name + "\nДата рождения: " + birthDate + "\nГруппа: " + groupName, keyboard
}

func (h *telegramHandler) changeClientArchive(ctx context.Context, id int64, archived bool) (string, models.InlineKeyboardMarkup) {
	err := setClientArchived(ctx, h.pool, id, archived)
	text, keyboard := h.clientCard(ctx, id)
	if err != nil {
		log.Printf("Change client archive failed: %v", err)
		text = "Не удалось изменить статус клиента. Попробуйте ещё раз.\n\n" + text
	}
	return text, keyboard
}

func (h *telegramHandler) handleClientInput(ctx context.Context, b *bot.Bot, chatID int64, value string, skip bool) {
	draft := h.clientDraft
	keyboard := models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "⏭️ Пропустить", CallbackData: "client_skip"}},
	}}
	if draft.name == "" {
		name := strings.TrimSpace(value)
		text := clientBirthDatePrompt
		if name == "" {
			text, keyboard = "Имя не должно быть пустым.\n"+clientNamePrompt, h.menu
		}
		if messageID, err := h.reply(ctx, b, chatID, text, keyboard); err != nil {
			log.Printf("Telegram sendMessage failed: %v", err)
		} else {
			draft.name, draft.promptID = name, messageID
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
		if err := createClient(ctx, h.pool, draft.name, birthDate, draft.groupID); err != nil {
			log.Printf("Create client failed: %v", err)
			text = "Не удалось сохранить клиента.\n" + clientBirthDatePrompt
		} else {
			h.clientDraft = nil
			text, keyboard = "Клиент создан.", h.menu
		}
	}
	if messageID, err := h.reply(ctx, b, chatID, text, keyboard); err != nil {
		log.Printf("Telegram sendMessage failed: %v", err)
	} else if h.clientDraft != nil {
		draft.promptID = messageID
	}
}
