package telegram

import (
	"capsule-me/internal/domain/catalog"
	"capsule-me/internal/domain/survey"
	"capsule-me/internal/usecase"
	"context"
	"errors"
	"log/slog"

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

	err := h.surveyService.AnswerQuestion(job.UserID, job.Data)
	if err != nil {
		if errors.Is(err, survey.ErrSurveyCompleted) ||
			errors.Is(err, survey.ErrAlreadyAnswered) ||
			errors.Is(err, survey.ErrInvalidAnswer) ||
			errors.Is(err, survey.ErrUnknownQuestion) {

			if errors.Is(err, survey.ErrSurveyCompleted) {
				_ = h.sendText(job, "Опрос уже завершён. Нажми /start чтобы начать заново.")
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

	res, err := h.catalogService.Recommend(ctx, feature)
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

	phCFG := bot.NewPhoto(job.ChatID, bot.FileBytes{Bytes: res.Image})
	phCFG.Caption = "твоя капсула!\nХочешь еще? -> жми /start"
	_, err = h.bot.Send(phCFG)
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
