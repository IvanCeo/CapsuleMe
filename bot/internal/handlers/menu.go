package handlers

import (
	"context"
	"fmt"
	"bot/internal/storage"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func handleMenu(ctx context.Context, b *bot.Bot, update *models.Update, db *storage.Database) {
	userID := update.CallbackQuery.From.ID

	// костыль на время отсутствия модуля сбора образов
	// начало костыля
	user, err := db.GetUser(ctx, userID)
	if err != nil {
		fmt.Errorf("Ошибка: %v", err)
	}

	if user.State == "done" {
		sendQuestionWithButtons(ctx, b, db, sendQuestionWithButtonsOptions{
			UserID: userID,
			QuestionID: 0,
			AnswerData: "",
			NextState: "aborted",
			Text: "Твой образ: ...(временное решение)\nПройти опрос еще?",
			Buttons: [][]models.InlineKeyboardButton{
				{{Text: "Да!", CallbackData: "qstart"}},
			},
		})
		return
	}
	// конец костыля
	
	// что-то тут происходит с определением стадии опроса прерванного
	last, err := db.GetLastAnswerByUser(ctx, userID)
	if err != nil {
		fmt.Errorf("Не нашел последнего ответа: %v", err)
		return
	}

	sendQuestionWithButtons(ctx, b, db, sendQuestionWithButtonsOptions{
		UserID: userID,
		QuestionID: 0,
		AnswerData: "",
		NextState: "aborted",
		Text: "Выбери",
		Buttons: [][]models.InlineKeyboardButton{
			{{Text: "Продолжить опрос", CallbackData: last.Answer}},
			{{Text: "Проийти заново", CallbackData: "qstart"}},
		},
	})
}