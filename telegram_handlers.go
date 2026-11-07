package main

import (
	"context"
	"errors"
	"log"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const groupNamePrompt = "Введите название группы. Для отмены — /start."

type telegramHandler struct {
	ownerID             int64
	pool                *pgxpool.Pool
	menu                models.InlineKeyboardMarkup
	waitingForGroupName atomic.Bool
	clientGroupID       int64
	clientName          string
	clientPromptID      int
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
	var groupID int64
	action, after, hasID := strings.Cut(query.Data, ":")
	if hasID && (action == "group" || action == "client") {
		id, err := strconv.ParseInt(after, 10, 64)
		if err != nil || id <= 0 {
			return
		}
		groupID = id
	} else if query.Data != "groups" && query.Data != "menu" && query.Data != "group_create" && query.Data != "client_skip" {
		return
	}
	if query.Data == "client_skip" && (h.clientGroupID == 0 || h.clientName == "" || message.ID != h.clientPromptID) {
		return
	}
	if query.Data != "client_skip" {
		h.clientGroupID, h.clientName, h.clientPromptID = 0, "", 0
	}
	if query.Data != "group_create" {
		h.waitingForGroupName.Store(false)
	}
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID}); err != nil {
		log.Printf("Telegram answerCallbackQuery failed: %v", err)
	}
	if query.Data == "client_skip" {
		h.handleClientInput(ctx, b, message.Chat.ID, "", true)
		return
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
		groups, err := listGroups(ctx, h.pool)
		switch {
		case err != nil:
			log.Printf("Load groups failed: %v", err)
			text = "Не удалось загрузить группы. Попробуйте ещё раз."
		case len(groups) == 0:
			text = "Групп пока нет"
		default:
			text = "Группы:"
			buttons := make([][]models.InlineKeyboardButton, 0, len(groups)+len(keyboard.InlineKeyboard))
			for _, group := range groups {
				buttons = append(buttons, []models.InlineKeyboardButton{{
					Text: group.Name, CallbackData: "group:" + strconv.FormatInt(group.ID, 10),
				}})
			}
			keyboard.InlineKeyboard = append(buttons, keyboard.InlineKeyboard...)
		}
	} else if groupID > 0 {
		keyboard = models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "Назад", CallbackData: "groups"}},
		}}
		name, err := getGroupName(ctx, h.pool, groupID)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			text = "Группа не найдена."
		case err != nil:
			log.Printf("Load group failed: %v", err)
			text = "Не удалось загрузить группу. Попробуйте ещё раз."
		default:
			text = name
			if action == "client" {
				if _, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: message.Chat.ID, Text: clientNamePrompt, ReplyMarkup: h.menu}); err != nil {
					log.Printf("Telegram sendMessage failed: %v", err)
				} else {
					h.clientGroupID = groupID
				}
				return
			}
			keyboard.InlineKeyboard = append([][]models.InlineKeyboardButton{
				{{Text: "Добавить клиента", CallbackData: "client:" + strconv.FormatInt(groupID, 10)}},
			}, keyboard.InlineKeyboard...)
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
		h.clientGroupID, h.clientName, h.clientPromptID = 0, "", 0
	} else if h.clientGroupID > 0 {
		h.handleClientInput(ctx, b, message.Chat.ID, message.Text, false)
		return
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
