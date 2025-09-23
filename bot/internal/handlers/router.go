package handlers

import (
	"context"
	"bot/internal/storage"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func Router(db *storage.Database) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		if update.Message != nil {
			handleStartMessage(ctx, b, update, db)
		}

		if update.CallbackQuery != nil {
			handleCallback(ctx, b, update, db)
		}
	}
}