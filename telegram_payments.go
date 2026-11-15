package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type paymentDraft struct {
	groupPayment
	clientName, groupName string
	saveKey               string
	step                  int
	saved                 bool
	pending               bool
}

func (h *telegramHandler) handlePayment(ctx context.Context, b *bot.Bot, chatID, id int64, action, input string) {
	var draft paymentDraft
	if h.paymentDraft != nil {
		draft = *h.paymentDraft
	} else if action != "payment" {
		return
	}
	problem := ""
	if action == "payment" && !draft.pending {
		draft = paymentDraft{groupPayment: groupPayment{ClientID: id}, saveKey: rand.Text()}
		c, err := getClient(ctx, h.pool, id)
		if err != nil {
			log.Printf("Load payment client failed: %v", err)
			problem = "Не удалось начать запись оплаты.\n\n"
		} else if c.GroupID == nil || c.GroupName == nil {
			problem = "Для групповой оплаты сначала выберите группу клиента.\n\n"
		} else {
			draft.GroupID, draft.clientName, draft.groupName, draft.step = *c.GroupID, c.Name, *c.GroupName, 1
		}
	} else if !draft.saved && (!draft.pending || action == "payment_save") {
		layout := "01.2006"
		if draft.step == 3 {
			layout = "02.01.2006"
		}
		if action == "payment_current" && (draft.step == 2 || draft.step == 3) {
			input, action = time.Now().In(time.FixedZone("MSK", 3*60*60)).Format(layout), "input"
		}
		switch action {
		case "input":
			input = strings.TrimSpace(input)
			if draft.step == 1 {
				amount, err := strconv.ParseInt(input, 10, 64)
				if err == nil && amount > 0 {
					draft.AmountRub, draft.step = amount, 2
				} else {
					problem = "Введите положительную целую сумму.\n"
				}
			} else if draft.step == 2 || draft.step == 3 {
				date, err := time.Parse(layout, input)
				if err != nil || date.Year() < 1 {
					problem = "Некорректная дата.\n"
				} else {
					if draft.step == 2 {
						draft.TrainingMonth = date
					} else {
						draft.ReceivedOn = date
					}
					draft.step++
				}
			} else {
				return
			}
		case "payment_back":
			draft.step--
		case "payment_save":
			if draft.step != 4 {
				return
			}
			if err := createGroupPayment(ctx, h.pool, draft.groupPayment, draft.saveKey); err != nil {
				log.Printf("Save group payment failed: %v", err)
				problem = "Не удалось сохранить оплату. Проверьте группу клиента и попробуйте ещё раз.\n\n"
				var pgError *pgconn.PgError
				definiteFailure := errors.Is(err, pgx.ErrNoRows) || pgconn.SafeToRetry(err) || (errors.As(err, &pgError) && pgError.SeverityUnlocalized == "ERROR")
				draft.pending = draft.pending || !definiteFailure
			} else {
				draft.saved = true
				draft.pending = false
			}
			h.paymentDraft = &draft
		default:
			return
		}
	}
	text := ""
	keyboard := models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "⬅️ Назад", CallbackData: "payment_back"}},
	}}
	if draft.step == 0 || draft.saved {
		text, keyboard = h.clientCard(ctx, draft.ClientID)
		if draft.saved {
			problem = "Оплата сохранена.\n\n"
		}
		if h.search != nil {
			keyboard.InlineKeyboard[len(keyboard.InlineKeyboard)-1][0].CallbackData = "search_results"
		}
	} else {
		switch draft.step {
		case 1:
			text = "Введите сумму оплаты."
		case 2, 3:
			text, input = "Введите месяц занятий (ММ.ГГГГ) или выберите текущий месяц.", "📅 Текущий месяц"
			if draft.step == 3 {
				text, input = "Введите дату получения денег (ДД.ММ.ГГГГ) или выберите сегодня.", "📅 Сегодня"
			}
			keyboard.InlineKeyboard = append([][]models.InlineKeyboardButton{{{Text: input, CallbackData: "payment_current"}}}, keyboard.InlineKeyboard...)
		case 4:
			text = fmt.Sprintf("Проверьте оплату:\nКлиент: %s\nГруппа: %s\nСумма: %d ₽\nМесяц занятий: %s\nДата получения денег: %s", draft.clientName, draft.groupName, draft.AmountRub, draft.TrainingMonth.Format("01.2006"), draft.ReceivedOn.Format("02.01.2006"))
			keyboard.InlineKeyboard = append([][]models.InlineKeyboardButton{{{Text: "💾 Сохранить", CallbackData: "payment_save"}}}, keyboard.InlineKeyboard...)
			if draft.pending {
				problem = "Не удалось подтвердить сохранение оплаты. Нажмите «Проверить сохранение», чтобы повторить попытку без дублирования.\n\n"
				keyboard.InlineKeyboard = keyboard.InlineKeyboard[:1]
				keyboard.InlineKeyboard[0][0].Text = "🔄 Проверить сохранение"
			}
		}
	}
	if _, err := h.reply(ctx, b, chatID, problem+text, keyboard); err != nil {
		log.Printf("Telegram payment reply failed: %v", err)
		return
	}
	h.paymentDraft = nil
	if draft.step != 0 && !draft.saved {
		h.paymentDraft = &draft
	}
}
