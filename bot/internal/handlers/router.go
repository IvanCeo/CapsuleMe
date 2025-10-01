package handlers

import (
	"context"
	"bot/internal/storage"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

var prefixMap = map[string]func(ctx context.Context, b *bot.Bot, update *models.Update, db *storage.Database){
    "m": handleMenu,
    "q": handleSurvey,
}

func selectPrefix(s string) (string) {
	return string([]rune(s)[0])
}

func Router(db *storage.Database) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		if update.Message != nil {
			handleStartMessage(ctx, b, update, db)
		}

		if update.CallbackQuery != nil {
			prefix := selectPrefix(update.CallbackQuery.Data)
			if handler, ok := prefixMap[prefix]; ok {
				handler(ctx, b, update, db)
			}
		}
	}
}