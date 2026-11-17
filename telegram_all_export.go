package main

import (
	"bytes"
	"context"
	"log"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

func allPaymentsExcel(ctx context.Context, pool *pgxpool.Pool) ([]byte, error) {
	groups, err := groupPaymentRows(ctx, pool, 0, "", nil)
	if err != nil {
		return nil, err
	}
	personal, err := personalPaymentRows(ctx, pool, 0, "", nil)
	if err != nil {
		return nil, err
	}
	return paymentWorkbook(paymentSheet{"Групповые", "E", groups}, paymentSheet{"Персональные", "C", personal})
}

func (h *telegramHandler) handleAllPaymentsExport(ctx context.Context, b *bot.Bot, chatID int64) {
	data, err := allPaymentsExcel(ctx, h.pool)
	if err == nil {
		_, err = b.SendDocument(ctx, &bot.SendDocumentParams{ChatID: chatID, Document: &models.InputFileUpload{
			Filename: "all-payments.xlsx", Data: bytes.NewReader(data),
		}})
	}
	if err != nil {
		log.Printf("Export all payments failed: %v", err)
		if _, err := h.reply(ctx, b, chatID, "Не удалось выгрузить оплаты. Попробуйте ещё раз.", h.menu); err != nil {
			log.Printf("Telegram export reply failed: %v", err)
		}
	}
}
