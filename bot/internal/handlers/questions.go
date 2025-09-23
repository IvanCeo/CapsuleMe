package handlers

import (
	"context"
	"fmt"
	"bot/internal/storage"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func sendQuestion(ctx context.Context, b *bot.Bot, db *storage.Database, chatID int64, userID int64, questionID int, answerData string, nextState string, text string, buttons [][]models.InlineKeyboardButton) {
	if questionID > 0 {
		if err := db.AddAnswer(ctx, userID, questionID, answerData); err != nil {
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   "Ошибка при сохранении ответа",
			})
			return
		}
	}

	if nextState != "" {
		db.UpdateUserState(ctx, userID, nextState)
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   text,
		ReplyMarkup: &models.InlineKeyboardMarkup{
			InlineKeyboard: buttons,
		},
	})
}

func handleCallback(ctx context.Context, b *bot.Bot, update *models.Update, db *storage.Database) {
	userID := update.CallbackQuery.From.ID
	chatID := userID // используем userID для приватного чата
	data := update.CallbackQuery.Data

	user, _ := db.GetUser(ctx, userID)
	if user == nil {
		db.CreateUser(ctx, &storage.User{ID: userID, State: "new"})
		user, _ = db.GetUser(ctx, userID)
	}

	switch data {
	case "start":
		sendQuestion(ctx, b, db, chatID, userID, 0, "", "q1",
			"выбери пол",
			[][]models.InlineKeyboardButton{
				{{Text: "мужчина", CallbackData: "q1_a"}},
				{{Text: "женщина", CallbackData: "q1_b"}},
			})
	case "q1_a", "q1_b":
		sendQuestion(ctx, b, db, chatID, userID, 1, data, "q2",
			"какую одежду вы хотите видеть в капсуле в первую очередь?",
			[][]models.InlineKeyboardButton{
				{{Text: "на каждый день", CallbackData: "q2_a"}},
				{{Text: "для спорта", CallbackData: "q2_b"}},
				{{Text: "для офиса", CallbackData: "q2_c"}},
				{{Text: "для праздника", CallbackData: "q2_d"}},
				{{Text: "для путешествий и отпуска", CallbackData: "q2_e"}},
				{{Text: "не имеет значения", CallbackData: "q2_f"}},
			})
	case "q2_a", "q2_b", "q2_c", "q2_d", "q2_e", "q2_f":
		sendQuestion(ctx, b, db, chatID, userID, 2, data, "q3",
			"на какое время года вы выбираете капсулу?",
			[][]models.InlineKeyboardButton{
				{{Text: "зима", CallbackData: "q3_a"}},
				{{Text: "весна", CallbackData: "q3_b"}},
				{{Text: "лето", CallbackData: "q3_c"}},
				{{Text: "осень", CallbackData: "q3_d"}},
			})
	case "q3_a", "q3_b", "q3_c", "q3_d":
		sendQuestion(ctx, b, db, chatID, userID, 3, data, "q4",
			"какой стиль вас интересует в первую очередь?",
			[][]models.InlineKeyboardButton{
				{{Text: "классический", CallbackData: "q4_a"}},
				{{Text: "спортивный", CallbackData: "q4_b"}},
				{{Text: "повседневный", CallbackData: "q4_c"}},
				{{Text: "уличный", CallbackData: "q4_d"}},
				{{Text: "романтический", CallbackData: "q4_e"}},
				{{Text: "смарт-кэжуал", CallbackData: "q4_f"}},
				{{Text: "элегантный", CallbackData: "q4_g"}},
				{{Text: "эклектика", CallbackData: "q4_h"}},
				{{Text: "минимализм", CallbackData: "q4_i"}},
				{{Text: "винтаж", CallbackData: "q4_j"}},
			})
	default:
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   fmt.Sprintf("Опрос успешно пройден!"),
		})
	}
}
