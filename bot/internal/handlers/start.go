package handlers

import (
	"context"
	"bot/internal/storage"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func handleStartMessage(ctx context.Context, b *bot.Bot, update *models.Update, db *storage.Database) {
	userID := update.Message.From.ID

	// получаем пользователя из БД или создаём нового
	user, _ := db.GetUser(ctx, userID)
	if user == nil {
		db.CreateUser(ctx, &storage.User{
			ID:    userID,
			State: "new",
		})
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "Привет! Жми кнопку, чтобы начать",
		ReplyMarkup: &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: "Начать", CallbackData: "start"}},
			},
		},
	})
}
