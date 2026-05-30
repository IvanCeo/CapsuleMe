package telegram

import (
	"capsule-me/internal/domain/catalog"
	"capsule-me/internal/domain/survey"
	"capsule-me/internal/usecase"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"

	bot "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type JobHandler struct {
	bot            *bot.BotAPI
	surveyService  *usecase.SurveyService
	catalogService *catalog.Recommender
	log            *slog.Logger
}

func NewJobHandler(
	bot *bot.BotAPI,
	surveyService *usecase.SurveyService,
	catalogService *catalog.Recommender,
	log *slog.Logger,
) *JobHandler {
	return &JobHandler{
		bot:            bot,
		surveyService:  surveyService,
		catalogService: catalogService,
		log:            log,
	}
}

func (h *JobHandler) Handle(ctx context.Context, job Job) {
	select {
	case <-ctx.Done():
		return
	default:
	}

	switch job.Type {
	case JobStart:
		h.handleStart(ctx, job)
	case JobAnswer:
		h.handleAnswer(ctx, job)
	}
}

func (h *JobHandler) handleStart(ctx context.Context, job Job) {
	_ = ctx

	question, err := h.surveyService.StartSurvey(job.UserID)
	if err != nil {
		h.log.Error(
			"job failed",
			"job id", job.JobID,
			"job type", job.Type,
			"chat id", job.ChatID,
			"user id", job.UserID,
			"update id", job.UpdateID,
			"err", err,
		)
		return
	}

	var rows [][]bot.InlineKeyboardButton
	for _, opt := range question.Options {
		row := []bot.InlineKeyboardButton{
			bot.NewInlineKeyboardButtonData(opt.Label, opt.Value),
		}
		rows = append(rows, row)
	}

	keyboard := bot.InlineKeyboardMarkup{
		InlineKeyboard: rows,
	}

	msg := bot.NewMessage(job.ChatID, question.Text)

	msg.ReplyMarkup = keyboard
	if _, err = h.bot.Send(msg); err != nil {
		h.log.Error(
			"job failed",
			"job id", job.JobID,
			"job type", job.Type,
			"chat id", job.ChatID,
			"user id", job.UserID,
			"update id", job.UpdateID,
			"err", err,
		)
		return
	}
}

func (h *JobHandler) handleAnswer(ctx context.Context, job Job) {
	h.log.Info(
		"job started",
		"job id", job.JobID,
		"job type", job.Type,
		"chat id", job.ChatID,
		"user id", job.UserID,
		"update id", job.UpdateID,
	)

	if job.CallbackQueryID != nil && *job.CallbackQueryID != "" {
		cb := bot.NewCallback(*job.CallbackQueryID, "")
		if _, err := h.bot.Request(cb); err != nil {
			h.log.Warn(
				"failed to answer callback query",
				"job id", job.JobID,
				"chat id", job.ChatID,
				"user id", job.UserID,
				"update id", job.UpdateID,
				"err", err,
			)
		}
	}

	if job.ChatID != 0 && job.MessageID != nil {
		del := bot.NewDeleteMessage(job.ChatID, *job.MessageID)
		if _, err := h.bot.Request(del); err != nil {
			h.log.Warn(
				"failed to delete message",
				"job id", job.JobID,
				"chat id", job.ChatID,
				"user id", job.UserID,
				"update id", job.UpdateID,
				"message id", *job.MessageID,
				"err", err,
			)
		}
	}

	if job.ChatID == 0 {
		h.log.Info("no chat id; stopping answer flow", "job id", job.JobID, "user id", job.UserID)
		return
	}

	select {
	case <-ctx.Done():
		h.log.Info("shutdown: stopping answer flow", "job id", job.JobID)
		return
	default:
	}

	if job.Data == "restart" {
		h.handleStart(ctx, job)
		return
	}

	// обратная связь будет рабоать иначе
	// тут хендлить ответы обратной связи
	// if job.Data[:1] == "0" || job.Data[:1] == "1" {
	// 	go func(job Job) {
	// 		key := job.Data[2:]
	// 		cpsl, err := h.surveyService.GetFromCache(ctx, key) // достатется по jobID из редиса
	// 		if err != nil {
	// 			h.log.Error("failed to get from cache", "err", err, "key from job", key, "job", job)
	// 			return
	// 		}
	// 		score := job.Data[:1]
	// 		err = h.surveyService.SaveFeedback(ctx, score, cpsl)
	// 		if err != nil {
	// 			h.log.Error("failed to save feedback", "err", err)
	// 			return
	// 		}
	// 	}(job)
	// }

	if job.Data[:4] == "show" {
		l, _ := strconv.Atoi(string(job.Data[5]))
		var wg sync.WaitGroup
		var rows [][]bot.InlineKeyboardButton
		for i := 0; i < l; i++ {
			wg.Add(1)
			go func(i int, id int64) {
				defer wg.Done()
				look, err := h.surveyService.GetLookByNumAndID(job.ChatID, i)
				if err != nil {
					// логгировать ошибку
					return
				}
				ph := bot.NewPhoto(id, bot.FileBytes{Bytes: look.Image})
				ph.Caption = "образ " + strconv.Itoa(i)
				_, _ = h.bot.Send(ph)
			}(i, job.ChatID)
			row := []bot.InlineKeyboardButton{
				bot.NewInlineKeyboardButtonData(fmt.Sprintf("нравится %d", i), fmt.Sprintf("like:%d", i)),
			}
			rows = append(rows, row)
		}
		wg.Wait()
		// дать клавиатуру что нравится
		rows = append(rows, []bot.InlineKeyboardButton{
			bot.NewInlineKeyboardButtonData("нрав все!", "like:100"),
			bot.NewInlineKeyboardButtonData("не нрав все", "like:-1"),
		})
		msg := bot.NewMessage(job.ChatID, "нравится?")
		msg.ReplyMarkup = bot.InlineKeyboardMarkup{
			InlineKeyboard: rows,
		}
		h.bot.Send(msg)
		return
	}

	if job.Data[:4] == "like" {
		mark, _ := strconv.Atoi(string(job.Data[5:]))
		switch {
		case mark == 100:
			// достать все луки по chatid
			// сохранить позитивно все лукки в постгрю
		case mark == -1:
			// достать все луки
			// сохранить негативно все луки в постгрю
		default:
			// достаь 1 лук
			// положить только его в постгрю
		}
		rows := [][]bot.InlineKeyboardButton{{
			bot.NewInlineKeyboardButtonData("следующая капсула", fmt.Sprintf("next:%v", job.ChatID)),
			bot.NewInlineKeyboardButtonData("пройти заново", "restart"),
		}}
		msg := bot.NewMessage(job.ChatID, "что дальше?")
		msg.ReplyMarkup = bot.InlineKeyboardMarkup{
			InlineKeyboard: rows,
		}
		if _, err := h.bot.Send(msg); err != nil {
			h.log.Error("failed to send keyboard", "err", err)
		}
		return
	}

	if job.Data[:4] == "next" {
		h.bot.Send(bot.NewMessage(job.ChatID, "секунду..."))
		go func(ctx context.Context, id int64) {
			feature, err := h.surveyService.GetIncomingFeatureByID(job.ChatID)
			if err != nil {
				h.log.Error("get incoming feature", "err", err)
				h.bot.Send(bot.NewMessage(job.ChatID, "сессия устарела, начини заново -> /start"))
				return
			}
			capsule, err := h.catalogService.Recommend(ctx, feature)
			if err != nil {
				h.log.Error("get recommend", "err", err)
				return
			}

			err = h.surveyService.SaveCapsule(job.ChatID, capsule)
			if err != nil {
				h.log.Error(
					"failed to save capsule",
					"job id", job.JobID,
					"chat id", job.ChatID,
					"user id", job.UserID,
					"update id", job.UpdateID,
					"err", err,
				)
				_ = h.sendText(job, "Произошла ошибка. Нажми /start") // на проде убрать
				return                                                // на проде убрать
			}

			looks, err := h.catalogService.RecommendLooks(ctx, capsule)
			if err != nil {
				h.log.Error(
					"failed to get look",
					"job id", job.JobID,
					"chat id", job.ChatID,
					"user id", job.UserID,
					"update id", job.UpdateID,
					"err", err,
				)
				_ = h.sendText(job, "Произошла ошибка. Нажми /start")
				return
			}

			err = h.surveyService.SaveLooks(job.ChatID, looks)
			if err != nil {
				h.log.Error(
					"failed to save looks",
					"job id", job.JobID,
					"chat id", job.ChatID,
					"user id", job.UserID,
					"update id", job.UpdateID,
					"err", err,
				)
				_ = h.sendText(job, "Произошла ошибка. Нажми /start") // на проде убрать
				return                                                // на проде убрать
			}

			phCFG := bot.NewPhoto(job.ChatID, bot.FileBytes{Bytes: capsule.Image})
			phCFG.Caption = "твоя капсула!\n"
			_, err = h.bot.Send(phCFG)

			keyboard := bot.InlineKeyboardMarkup{
				InlineKeyboard: [][]bot.InlineKeyboardButton{
					{bot.NewInlineKeyboardButtonData("показать образы", fmt.Sprintf("show:%v:%v", len(looks.Outfits), job.ChatID))},
					{bot.NewInlineKeyboardButtonData("следующая капсула", fmt.Sprintf("next:%v", job.ChatID))},
					{bot.NewInlineKeyboardButtonData("начать заново", "restart")},
				},
			}

			msg := bot.NewMessage(job.ChatID, "Что дальше?")
			msg.ReplyMarkup = keyboard
			h.bot.Send(msg)

		}(ctx, job.ChatID)
		return
	}

	err := h.surveyService.AnswerQuestion(job.UserID, job.Data)
	if err != nil {
		if errors.Is(err, survey.ErrSurveyCompleted) ||
			errors.Is(err, survey.ErrAlreadyAnswered) ||
			errors.Is(err, survey.ErrInvalidAnswer) ||
			errors.Is(err, survey.ErrUnknownQuestion) {

			if errors.Is(err, survey.ErrSurveyCompleted) {
				_ = h.sendText(job, "Жми -> /start ")
				return
			}

			q, qErr := h.surveyService.GetCurrentQuestion(job.UserID)
			if qErr == nil {
				_ = h.sendQuestion(job.ChatID, q)
				return
			}

			_ = h.sendText(job, "Сессия устарела. Нажми /start")
			return
		}

		h.log.Error("failed to answer question",
			"job id", job.JobID,
			"chat id", job.ChatID,
			"user id", job.UserID,
			"update id", job.UpdateID,
			"err", err,
		)
		_ = h.sendText(job, "Произошла ошибка. Нажми /start")
		return
	}

	done, err := h.surveyService.IsDone(job.UserID)
	if err != nil {
		h.log.Error(
			"failed to check isDone",
			"job id", job.JobID,
			"chat id", job.ChatID,
			"user id", job.UserID,
			"update id", job.UpdateID,
			"err", err,
		)
		_ = h.sendText(job, "Произошла ошибка. Нажми /start")
		return
	}

	if !done {
		question, err := h.surveyService.GetCurrentQuestion(job.UserID)
		if err != nil {
			h.log.Error(
				"failed to get current question",
				"job id", job.JobID,
				"chat id", job.ChatID,
				"user id", job.UserID,
				"update id", job.UpdateID,
				"err", err,
			)
			_ = h.sendText(job, "Произошла ошибка. Нажми /start")
			return
		}

		if err := h.sendQuestion(job.ChatID, question); err != nil {
			h.log.Error(
				"failed to send next question",
				"job id", job.JobID,
				"chat id", job.ChatID,
				"user id", job.UserID,
				"update id", job.UpdateID,
				"err", err,
			)
			return
		}
		return
	}

	h.bot.Send(bot.NewMessage(job.ChatID, "секунду..."))

	session, err := h.surveyService.GetSessionByUser(job.UserID)
	if err != nil {
		h.log.Error(
			"failed to get session",
			"job id", job.JobID,
			"chat id", job.ChatID,
			"user id", job.UserID,
			"update id", job.UpdateID,
			"err", err,
		)
		_ = h.sendText(job, "Произошла ошибка. Нажми /start")
		return
	}

	feature, err := h.surveyService.MapIncomingFeature(session)
	if err != nil {
		h.log.Error(
			"failed to map feature",
			"job id", job.JobID,
			"chat id", job.ChatID,
			"user id", job.UserID,
			"update id", job.UpdateID,
			"err", err,
		)
		_ = h.sendText(job, "Произошла ошибка. Нажми /start")
		return
	}

	// сохранить features
	err = h.surveyService.SaveIncomingFeature(job.ChatID, feature)
	if err != nil {
		h.log.Error(
			"failed to save feature",
			"job id", job.JobID,
			"chat id", job.ChatID,
			"user id", job.UserID,
			"update id", job.UpdateID,
			"err", err,
		)
		_ = h.sendText(job, "Произошла ошибка. Нажми /start") // на проде убрать
		return                                                // на проде убрать
	}

	// та самая которая кладет в канал job воркеров
	casule, err := h.catalogService.Recommend(ctx, feature)
	if err != nil {
		h.log.Error(
			"failed to Recommend",
			"job id", job.JobID,
			"chat id", job.ChatID,
			"user id", job.UserID,
			"update id", job.UpdateID,
			"err", err,
		)
		_ = h.sendText(job, "Произошла ошибка. Нажми /start")
		return
	}

	// сохранить каспсулу
	err = h.surveyService.SaveCapsule(job.ChatID, casule)
	if err != nil {
		h.log.Error(
			"failed to save capsule",
			"job id", job.JobID,
			"chat id", job.ChatID,
			"user id", job.UserID,
			"update id", job.UpdateID,
			"err", err,
		)
		_ = h.sendText(job, "Произошла ошибка. Нажми /start") // на проде убрать
		return                                                // на проде убрать
	}

	//получить лук
	looks, err := h.catalogService.RecommendLooks(ctx, casule)
	if err != nil {
		h.log.Error(
			"failed to get look",
			"job id", job.JobID,
			"chat id", job.ChatID,
			"user id", job.UserID,
			"update id", job.UpdateID,
			"err", err,
		)
		_ = h.sendText(job, "Произошла ошибка. Нажми /start")
		return
	}

	// сохранить лук
	err = h.surveyService.SaveLooks(job.ChatID, looks)
	if err != nil {
		h.log.Error(
			"failed to save looks",
			"job id", job.JobID,
			"chat id", job.ChatID,
			"user id", job.UserID,
			"update id", job.UpdateID,
			"err", err,
		)
		_ = h.sendText(job, "Произошла ошибка. Нажми /start") // на проде убрать
		return                                                // на проде убрать
	}

	// отпправить капсулу
	phCFG := bot.NewPhoto(job.ChatID, bot.FileBytes{Bytes: casule.Image})
	phCFG.Caption = "твоя капсула!\n"
	_, err = h.bot.Send(phCFG)

	keyboard := bot.InlineKeyboardMarkup{
		InlineKeyboard: [][]bot.InlineKeyboardButton{
			{bot.NewInlineKeyboardButtonData("показать образы", fmt.Sprintf("show:%v:%v", len(looks.Outfits), job.ChatID))},
			{bot.NewInlineKeyboardButtonData("следующая капсула", fmt.Sprintf("next:%v", job.ChatID))},
			{bot.NewInlineKeyboardButtonData("начать заново", "/start")},
		},
	}

	msg := bot.NewMessage(job.ChatID, "Что дальше?")
	msg.ReplyMarkup = keyboard
	h.bot.Send(msg)

	if err != nil {
		h.log.Error("ошибка отправки фото", "err", err)
		_ = h.sendText(job, "Произошла ошибка. Нажми /start")
		return
	} else {
		h.log.Info("фото успешно отправлено")
	}
}

func (h *JobHandler) sendText(job Job, text string) error {
	_, err := h.bot.Send(bot.NewMessage(job.ChatID, text))
	return err
}

func (h *JobHandler) sendQuestion(chatID int64, q *survey.Question) error {
	var rows [][]bot.InlineKeyboardButton

	for _, opt := range q.Options {
		rows = append(rows, []bot.InlineKeyboardButton{
			bot.NewInlineKeyboardButtonData(opt.Label, opt.Value),
		})
	}

	msg := bot.NewMessage(chatID, q.Text)
	msg.ReplyMarkup = bot.InlineKeyboardMarkup{
		InlineKeyboard: rows,
	}

	_, err := h.bot.Send(msg)
	return err
}
