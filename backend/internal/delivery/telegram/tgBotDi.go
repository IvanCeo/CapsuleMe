package telegram

import (
	adapter "capsule-me/internal/adapter/catalogML"
	"capsule-me/internal/domain/catalog"
	"capsule-me/internal/logger"
	"capsule-me/internal/usecase"
	"capsule-me/test/mocks"
	"log/slog"
	"os"

	bot "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Bot struct {
	Bot     *bot.BotAPI
	Handler *JobHandler
	Log     *slog.Logger
}

func NewBot() (*Bot, error) {
	bot, err := bot.NewBotAPI(os.Getenv("TG_TOKEN"))
	if err != nil {
		return nil, err
	}
	log := logger.New()

	catalogClient := adapter.NewCatalogClient(os.Getenv("OUTFIT_SERVICE_URL"))
	sessionRepo := mocks.NewSessionRepoMock()
	surveyService, err := usecase.NewSurveyService(sessionRepo, log)
	catalogService := catalog.NewRecommender(catalogClient)

	if err != nil {
		return nil, err
	}
	handler := NewJobHandler(bot, surveyService, catalogService, log)
	return &Bot{
		Bot:     bot,
		Handler: handler,
		Log:     log,
	}, nil
}
