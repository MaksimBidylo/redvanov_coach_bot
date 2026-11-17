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

const groupNamePrompt = "Введите название группы."

type telegramHandler struct {
	ownerID             int64
	pool                *pgxpool.Pool
	menu                models.InlineKeyboardMarkup
	waitingForGroupName atomic.Bool
	clientDraft         *clientDraft
	paymentDraft        *paymentDraft
	paymentHistory      *clientPaymentHistory
	paymentEdit         *paymentEdit
	groupSummary        *groupSummary
	renameGroupID       int64
	editClientID        int64
	editBirthDate       bool
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
	if (versioned || data == "client_skip" || data == "client_back" || strings.HasPrefix(data, "search_") || strings.HasPrefix(data, "move:") || strings.HasPrefix(data, "rename:") || strings.HasPrefix(data, "edit_") || strings.HasPrefix(data, "del") || strings.HasPrefix(data, "archive:") || strings.HasPrefix(data, "restore:") || strings.HasPrefix(data, "gar") || strings.HasPrefix(data, "group_restore:") || strings.HasPrefix(data, "payment") || strings.HasPrefix(data, "ledger") || strings.HasPrefix(data, "summary")) && revision != strconv.FormatInt(h.revision, 10) {
		return
	}
	query.Data = data
	var id, groupID int64
	action, after, hasID := strings.Cut(query.Data, ":")
	var hasTarget bool
	if action == "move" || action == "del" || action == "delc" || action == "gar" || action == "garc" {
		var target string
		after, target, hasTarget = strings.Cut(after, ":")
		if hasTarget {
			var err error
			groupID, err = strconv.ParseInt(target, 10, 64)
			if err != nil || groupID < 0 {
				return
			}
		}
	}
	if hasID && (action == "group" || action == "client" || action == "client_view" || action == "search_view" || action == "move" || action == "rename" || action == "edit_name" || action == "edit_birth" || action == "edit_date" || action == "edit_clear" || action == "del" || action == "delc" || action == "archive" || action == "restore" || action == "gar" || action == "garc" || action == "group_restore" || action == "payment" || action == "ledger" || action == "ledger_page" || action == "ledger_view" || action == "ledger_delete" || action == "ledger_confirm" || action == "ledger_edit" || action == "summary") {
		parsedID, err := strconv.ParseInt(after, 10, 64)
		if err != nil || parsedID < 0 || (parsedID == 0 && action != "client" && action != "ledger_page" && action != "ledger_view" && action != "ledger_delete" && action != "ledger_confirm" && action != "ledger_edit") {
			return
		}
		id = parsedID
	} else if query.Data != "groups" && query.Data != "archived_groups" && query.Data != "menu" && query.Data != "group_create" && query.Data != "client_skip" && query.Data != "client_back" && query.Data != "ungrouped" && query.Data != "search" && query.Data != "search_results" && query.Data != "payment_back" && query.Data != "payment_current" && query.Data != "payment_save" && query.Data != "payment_group" && query.Data != "payment_personal" && query.Data != "ledger_client" && query.Data != "ledger_edit_amount" && query.Data != "ledger_edit_received" && query.Data != "ledger_edit_month" && query.Data != "ledger_edit_back" && query.Data != "ledger_edit_save" && query.Data != "summary_month" && query.Data != "summary_all" && query.Data != "summary_back" {
		return
	}
	if (action == "delc" || action == "garc") && !hasTarget {
		return
	}
	if (action == "del" || action == "delc" || action == "gar" || action == "garc") && hasTarget && groupID == id {
		return
	}
	if (action == "search_view" || query.Data == "search_results") && (h.search == nil || h.search.messageID == 0) {
		return
	}
	if (query.Data == "client_skip" || query.Data == "client_back") && (h.clientDraft == nil || h.clientDraft.name == "" || message.ID != h.clientDraft.promptID) {
		return
	}
	h.messageID = message.ID
	h.renameGroupID = 0
	h.editClientID = 0
	if action != "search_view" && action != "move" && !strings.HasPrefix(action, "edit_") && !strings.HasPrefix(action, "payment") && !strings.HasPrefix(action, "ledger") && action != "archive" && action != "restore" && query.Data != "search_results" && (h.paymentDraft == nil || (!h.paymentDraft.pending && !h.paymentDraft.saved)) {
		h.search = nil
	}
	previousClientDraft := h.clientDraft
	if query.Data != "client_skip" && query.Data != "client_back" {
		h.clientDraft = nil
	}
	if query.Data != "group_create" {
		h.waitingForGroupName.Store(false)
	}
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID}); err != nil {
		log.Printf("Telegram answerCallbackQuery failed: %v", err)
	}
	if strings.HasPrefix(action, "payment") || (h.paymentDraft != nil && (h.paymentDraft.pending || h.paymentDraft.saved)) {
		h.handlePayment(ctx, b, message.Chat.ID, id, action, "")
		return
	}
	h.paymentDraft = nil
	if strings.HasPrefix(action, "summary") {
		h.handleGroupSummary(ctx, b, message.Chat.ID, id, action, "")
		return
	}
	if strings.HasPrefix(action, "ledger_edit") {
		h.handlePaymentEdit(ctx, b, message.Chat.ID, id, action, "")
		return
	}
	if strings.HasPrefix(action, "ledger") {
		h.handleClientPayments(ctx, b, message.Chat.ID, action, id)
		return
	}
	if query.Data == "search" {
		h.handleClientSearch(ctx, b, message.Chat.ID, "")
		return
	}
	if query.Data == "search_results" {
		h.handleClientSearch(ctx, b, message.Chat.ID, h.search.query)
		return
	}
	if query.Data == "client_skip" || query.Data == "client_back" {
		h.handleClientInput(ctx, b, message.Chat.ID, "", query.Data)
		return
	}
	if query.Data == "group_create" {
		keyboard := models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "⬅️ Назад", CallbackData: "groups"}},
		}}
		if _, err := h.reply(ctx, b, message.Chat.ID, groupNamePrompt, keyboard); err != nil {
			log.Printf("Telegram sendMessage failed: %v", err)
		} else {
			h.waitingForGroupName.Store(true)
		}
		return
	}
	if strings.HasPrefix(action, "edit_") {
		h.handleClientEdit(ctx, b, message.Chat.ID, id, action, "")
		return
	}
	text, keyboard := "Добро пожаловать!", h.menu
	if action == "archive" || action == "restore" {
		text, keyboard = h.changeClientArchive(ctx, id, action == "archive")
	} else if action == "del" || action == "delc" || action == "gar" || action == "garc" {
		text, keyboard = h.closeGroupMenu(ctx, id, groupID, hasTarget, action == "delc" || action == "garc", strings.HasPrefix(action, "gar"))
	} else if action == "move" {
		text, keyboard = h.changeClientGroup(ctx, id, groupID, hasTarget)
	} else if action == "client_view" || action == "search_view" {
		text, keyboard = h.clientCard(ctx, id)
	} else if query.Data == "ungrouped" {
		body, buttons := h.groupClientsMenu(ctx, nil)
		text = "Клиенты без группы\n\n" + body
		keyboard = models.InlineKeyboardMarkup{InlineKeyboard: append(buttons,
			[]models.InlineKeyboardButton{{Text: "➕ Добавить клиента", CallbackData: "client:0"}},
			[]models.InlineKeyboardButton{{Text: "⬅️ Назад", CallbackData: "menu"}},
		)}
	} else if query.Data == "groups" || query.Data == "archived_groups" {
		archived := query.Data == "archived_groups"
		keyboard = models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "➕ Добавить группу", CallbackData: "group_create"}},
			{{Text: "📦 Архив групп", CallbackData: "archived_groups"}},
			{{Text: "⬅️ Назад", CallbackData: "menu"}},
		}}
		if archived {
			keyboard.InlineKeyboard = keyboard.InlineKeyboard[len(keyboard.InlineKeyboard)-1:]
			keyboard.InlineKeyboard[0][0].CallbackData = "groups"
		}
		groups, err := listGroups(ctx, h.pool, archived)
		switch {
		case err != nil:
			log.Printf("Load groups failed: %v", err)
			text = "Не удалось загрузить группы. Попробуйте ещё раз."
		case len(groups) == 0:
			text = "Групп пока нет"
		default:
			text = "Группы:"
			if archived {
				text = "Архив групп:"
			}
			buttons := make([][]models.InlineKeyboardButton, 0, len(groups)+len(keyboard.InlineKeyboard))
			for _, group := range groups {
				label, action := group.Name, "group:"
				if archived {
					label, action = "♻️ Восстановить «"+group.Name+"»", "group_restore:"
				}
				buttons = append(buttons, []models.InlineKeyboardButton{{
					Text: label, CallbackData: action + strconv.FormatInt(group.ID, 10),
				}})
			}
			keyboard.InlineKeyboard = append(buttons, keyboard.InlineKeyboard...)
		}
	} else if id > 0 || action == "client" {
		keyboard = models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "⬅️ Назад", CallbackData: "groups"}},
		}}
		var name string
		var err error
		if action == "group_restore" {
			name, err = restoreGroup(ctx, h.pool, id)
			keyboard.InlineKeyboard[0][0].CallbackData = "archived_groups"
		} else if id > 0 {
			name, err = getGroupName(ctx, h.pool, id)
		}
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			text = "Группа не найдена."
		case err != nil:
			log.Printf("Load group failed: %v", err)
			text = "Не удалось загрузить группу. Попробуйте ещё раз."
			if action == "group_restore" {
				text = "Не удалось восстановить группу. Попробуйте ещё раз."
			}
		default:
			text = name
			if action == "rename" {
				keyboard.InlineKeyboard[0][0].CallbackData = "group:" + strconv.FormatInt(id, 10)
				if _, err := h.reply(ctx, b, message.Chat.ID, groupRenamePrompt, keyboard); err != nil {
					log.Printf("Telegram reply failed: %v", err)
				} else {
					h.renameGroupID = id
				}
				return
			}
			if action == "client" {
				draft := &clientDraft{}
				if id > 0 {
					draft.groupID = &id
				}
				keyboard.InlineKeyboard[0][0].CallbackData = draft.backCallback() + "|" + strconv.FormatInt(h.revision+1, 10)
				if messageID, err := h.reply(ctx, b, message.Chat.ID, clientNamePrompt, keyboard); err != nil {
					log.Printf("Telegram sendMessage failed: %v", err)
				} else {
					draft.promptID, h.clientDraft = messageID, draft
				}
				return
			}
			text, keyboard = h.groupCard(ctx, id, name)
		}
	}
	if h.search != nil && (action == "search_view" || action == "archive" || action == "restore" || (action == "move" && hasTarget)) {
		keyboard.InlineKeyboard[len(keyboard.InlineKeyboard)-1][0].CallbackData = "search_results"
	}
	if _, err := h.reply(ctx, b, message.Chat.ID, text, keyboard); err != nil {
		if previousClientDraft != nil && previousClientDraft.name == "" && query.Data == previousClientDraft.backCallback() {
			h.clientDraft = previousClientDraft
		}
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
	if message.Text == "/start" && h.paymentDraft != nil && (h.paymentDraft.pending || h.paymentDraft.saved) {
		h.handlePayment(ctx, b, message.Chat.ID, 0, "input", message.Text)
		return
	}
	if message.Text == "/start" {
		h.waitingForGroupName.Store(false)
		h.clientDraft = nil
		h.search = nil
		h.renameGroupID = 0
		h.editClientID = 0
		if _, err := h.reply(ctx, b, message.Chat.ID, "Добро пожаловать!", h.menu); err != nil {
			log.Printf("Telegram sendMessage failed: %v", err)
		} else {
			h.paymentDraft = nil
			h.paymentEdit = nil
		}
		return
	}
	if h.paymentDraft != nil {
		replyExpected = h.paymentDraft.saved || (h.paymentDraft.step > 0 && h.paymentDraft.step < 4)
		h.handlePayment(ctx, b, message.Chat.ID, 0, "input", message.Text)
		return
	} else if h.paymentEdit != nil {
		replyExpected = h.paymentEdit.field != "" && h.paymentEdit.value == nil
		h.handlePaymentEdit(ctx, b, message.Chat.ID, 0, "input", message.Text)
		return
	} else if h.groupSummary != nil && h.groupSummary.waitingMonth {
		h.handleGroupSummary(ctx, b, message.Chat.ID, 0, "input", message.Text)
		return
	} else if h.renameGroupID != 0 {
		h.handleGroupRename(ctx, b, message.Chat.ID, message.Text)
		return
	} else if h.editClientID != 0 {
		h.handleClientEdit(ctx, b, message.Chat.ID, h.editClientID, "input", message.Text)
		return
	} else if h.search != nil && h.search.messageID == 0 {
		h.handleClientSearch(ctx, b, message.Chat.ID, message.Text)
		return
	} else if h.clientDraft != nil {
		h.handleClientInput(ctx, b, message.Chat.ID, message.Text, "input")
		return
	}

	if !h.waitingForGroupName.Load() {
		replyExpected = false
		return
	}
	name := strings.TrimSpace(message.Text)
	text := "Название не должно быть пустым.\n" + groupNamePrompt
	keyboard := models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "⬅️ Назад", CallbackData: "groups"}},
	}}
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
			keyboard = h.menu
		}
	}
	_, err := h.reply(ctx, b, message.Chat.ID, text, keyboard)
	if err != nil {
		log.Printf("Telegram sendMessage failed: %v", err)
	}
}
