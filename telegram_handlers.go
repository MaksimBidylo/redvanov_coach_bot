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
	clientDraft         *clientDraft
	search              *clientSearch
	messageID           int
	revision            int64
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
	if query.From.ID != h.ownerID || message == nil || message.Chat.Type != models.ChatTypePrivate || (h.messageID != 0 && message.ID != h.messageID) {
		return
	}
	data, revision, versioned := strings.Cut(query.Data, "|")
	if (versioned || data == "client_skip" || strings.HasPrefix(data, "search_")) && revision != strconv.FormatInt(h.revision, 10) {
		return
	}
	query.Data = data
	var id int64
	action, after, hasID := strings.Cut(query.Data, ":")
	if hasID && (action == "group" || action == "client" || action == "client_view" || action == "search_view") {
		parsedID, err := strconv.ParseInt(after, 10, 64)
		if err != nil || parsedID < 0 || (parsedID == 0 && action != "client") {
			return
		}
		id = parsedID
	} else if query.Data != "groups" && query.Data != "menu" && query.Data != "group_create" && query.Data != "client_skip" && query.Data != "ungrouped" && query.Data != "search" && query.Data != "search_results" {
		return
	}
	if (action == "search_view" || query.Data == "search_results") && (h.search == nil || h.search.messageID == 0) {
		return
	}
	if query.Data == "client_skip" && (h.clientDraft == nil || h.clientDraft.name == "" || message.ID != h.clientDraft.promptID) {
		return
	}
	h.messageID = message.ID
	if action != "search_view" && query.Data != "search_results" {
		h.search = nil
	}
	if query.Data != "client_skip" {
		h.clientDraft = nil
	}
	if query.Data != "group_create" {
		h.waitingForGroupName.Store(false)
	}
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID}); err != nil {
		log.Printf("Telegram answerCallbackQuery failed: %v", err)
	}
	if query.Data == "search" {
		h.handleClientSearch(ctx, b, message.Chat.ID, "")
		return
	}
	if query.Data == "search_results" {
		h.handleClientSearch(ctx, b, message.Chat.ID, h.search.query)
		return
	}
	if query.Data == "client_skip" {
		h.handleClientInput(ctx, b, message.Chat.ID, "", true)
		return
	}
	if query.Data == "group_create" {
		if _, err := h.reply(ctx, b, message.Chat.ID, groupNamePrompt, h.menu); err != nil {
			log.Printf("Telegram sendMessage failed: %v", err)
		} else {
			h.waitingForGroupName.Store(true)
		}
		return
	}
	text, keyboard := "Добро пожаловать!", h.menu
	if action == "client_view" || action == "search_view" {
		text, keyboard = h.clientCard(ctx, id)
		if action == "search_view" {
			keyboard.InlineKeyboard[0][0].CallbackData = "search_results"
		}
	} else if query.Data == "ungrouped" {
		body, buttons := h.groupClientsMenu(ctx, nil)
		text = "Клиенты без группы\n\n" + body
		keyboard = models.InlineKeyboardMarkup{InlineKeyboard: append(buttons,
			[]models.InlineKeyboardButton{{Text: "Добавить клиента", CallbackData: "client:0"}},
			[]models.InlineKeyboardButton{{Text: "Назад", CallbackData: "menu"}},
		)}
	} else if query.Data == "groups" {
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
	} else if id > 0 || action == "client" {
		keyboard = models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "Назад", CallbackData: "groups"}},
		}}
		var name string
		var err error
		if id > 0 {
			name, err = getGroupName(ctx, h.pool, id)
		}
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			text = "Группа не найдена."
		case err != nil:
			log.Printf("Load group failed: %v", err)
			text = "Не удалось загрузить группу. Попробуйте ещё раз."
		default:
			text = name
			if action == "client" {
				if _, err := h.reply(ctx, b, message.Chat.ID, clientNamePrompt, h.menu); err != nil {
					log.Printf("Telegram sendMessage failed: %v", err)
				} else {
					h.clientDraft = &clientDraft{}
					if id > 0 {
						h.clientDraft.groupID = &id
					}
				}
				return
			}
			keyboard.InlineKeyboard = append([][]models.InlineKeyboardButton{
				{{Text: "Добавить клиента", CallbackData: "client:" + strconv.FormatInt(id, 10)}},
			}, keyboard.InlineKeyboard...)
			body, buttons := h.groupClientsMenu(ctx, &id)
			text += "\n\n" + body
			keyboard.InlineKeyboard = append(buttons, keyboard.InlineKeyboard...)
		}
	}
	if _, err := h.reply(ctx, b, message.Chat.ID, text, keyboard); err != nil {
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
	revision := h.revision
	replyExpected := true
	defer func() {
		if replyExpected && h.revision == revision {
			return
		}
		if _, err := b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: message.Chat.ID, MessageID: message.ID}); err != nil {
			log.Printf("Telegram deleteMessage failed: %v", err)
		}
	}()
	text := "Добро пожаловать!"
	if message.Text == "/start" {
		h.waitingForGroupName.Store(false)
		h.clientDraft = nil
		h.search = nil
	} else if h.search != nil && h.search.messageID == 0 {
		h.handleClientSearch(ctx, b, message.Chat.ID, message.Text)
		return
	} else if h.clientDraft != nil {
		h.handleClientInput(ctx, b, message.Chat.ID, message.Text, false)
		return
	} else {
		if !h.waitingForGroupName.Load() {
			replyExpected = false
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
	_, err := h.reply(ctx, b, message.Chat.ID, text, h.menu)
	if err != nil {
		log.Printf("Telegram sendMessage failed: %v", err)
	}
}
