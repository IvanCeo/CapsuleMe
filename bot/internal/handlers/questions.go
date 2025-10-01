package handlers

import (
	"context"
	// "fmt"
	"bot/internal/storage"
	internalModels "bot/internal/models"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type sendQuestionWithButtonsOptions struct {
	UserID int64
	QuestionID int // нужно лишь чтобы добавлять ответ в бд, если вопрос не по опросу, ставить 0
	AnswerData string // тоже служебное поле, чтобы писать в бд, можно оставлять пустым
	NextState string
	Text string
	Buttons [][]models.InlineKeyboardButton
}

func sendQuestionWithButtons(ctx context.Context, b *bot.Bot, db *storage.Database, opt sendQuestionWithButtonsOptions) {
	u := &internalModels.User{
		ID: opt.UserID,
		State: opt.NextState,
	}
	a := &internalModels.Answer{
		UserID: opt.UserID,
		QuestionID: opt.QuestionID,
		Answer: opt.AnswerData,
	}
	if opt.QuestionID > 0 {
		if err := db.AddAnswer(ctx, a); err != nil {
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: a.UserID,
				Text:   "Ошибка при сохранении ответа",
			})
			return
		}
	}

	if opt.NextState != "" {
		db.UpdateUserState(ctx, u)
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: opt.UserID,
		Text:   opt.Text,
		ReplyMarkup: &models.InlineKeyboardMarkup{
			InlineKeyboard: opt.Buttons,
		},
	})
}

func handleSurvey(ctx context.Context, b *bot.Bot, update *models.Update, db *storage.Database) {
	userID := update.CallbackQuery.From.ID
	data := update.CallbackQuery.Data

	user, _ := db.GetUser(ctx, userID)
	if user == nil {
		db.CreateUser(ctx, &internalModels.User{ID: userID, State: "new"})
		user, _ = db.GetUser(ctx, userID)
	}

	switch data {

	case "qstart":
		sendQuestionWithButtons(ctx, b, db, sendQuestionWithButtonsOptions{
			UserID: userID,
			QuestionID: 0,
			AnswerData: "",
			NextState: "q1",
			Text: "выбери пол",
			Buttons: [][]models.InlineKeyboardButton{
				{{Text: "мужчина", CallbackData: "q1_a"}},
				{{Text: "женщина", CallbackData: "q1_b"}},
				{{Text: "стоп, в меню", CallbackData: "m"}},
			},
		})
	case "q1_a", "q1_b":
		sendQuestionWithButtons(ctx, b, db, sendQuestionWithButtonsOptions{
			UserID: userID,
			QuestionID: 1,
			AnswerData: data,
			NextState: "q2",
			Text: "какую одежду вы хотите видеть в капсуле в первую очередь?",
			Buttons: [][]models.InlineKeyboardButton{
				{{Text: "на каждый день", CallbackData: "q2_a"}},
				{{Text: "для спорта", CallbackData: "q2_b"}},
				{{Text: "для офиса", CallbackData: "q2_c"}},
				{{Text: "для праздника", CallbackData: "q2_d"}},
				{{Text: "для путешествий и отпуска", CallbackData: "q2_e"}},
				{{Text: "не имеет значения", CallbackData: "q2_f"}},
				{{Text: "стоп, в меню", CallbackData: "m"}},
			},
		})
	case "q2_a", "q2_b", "q2_c", "q2_d", "q2_e", "q2_f":
		sendQuestionWithButtons(ctx, b, db, sendQuestionWithButtonsOptions{
			UserID: userID,
			QuestionID: 2,
			AnswerData: data,
			NextState: "q3",
			Text: "на какое время года вы выбираете капсулу?",
			Buttons: [][]models.InlineKeyboardButton{
				{{Text: "зима", CallbackData: "q3_a"}},
				{{Text: "весна", CallbackData: "q3_b"}},
				{{Text: "лето", CallbackData: "q3_c"}},
				{{Text: "осень", CallbackData: "q3_d"}},
				{{Text: "стоп, в меню", CallbackData: "m"}},
			},
		})
	case "q3_a", "q3_b", "q3_c", "q3_d":
		sendQuestionWithButtons(ctx, b, db, sendQuestionWithButtonsOptions{
			UserID: userID,
			QuestionID: 3,
			AnswerData: data,
			NextState: "q4",
			Text: "какой стиль вас интересует в первую очередь?",
			Buttons: [][]models.InlineKeyboardButton{
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
				{{Text: "стоп, в меню", CallbackData: "m"}},
			},
		})
	default:
		sendQuestionWithButtons(ctx, b, db, sendQuestionWithButtonsOptions{
			UserID: userID,
			QuestionID: 0,
			AnswerData: data,
			NextState: "done",
			Text: "собрать образ!",
			Buttons: [][]models.InlineKeyboardButton{
				{{Text: "собрать!", CallbackData: "m"}}, // пока нет модуля сбора, просто возвращаем в меню
				{{Text: "стоп, в меню", CallbackData: "m"}},
			},
		})
	}
}