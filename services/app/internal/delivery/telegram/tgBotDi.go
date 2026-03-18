package telegram

import (
	"capsule-me/internal/delivery/grpc"
	"capsule-me/internal/domain/catalog"
	"capsule-me/internal/logger"
	"capsule-me/internal/usecase"
	"capsule-me/test/mocks"
	"log/slog"

	bot "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Bot struct {
	Bot     *bot.BotAPI
	Handler *JobHandler
	Log     *slog.Logger
}

func NewBot(catalogClient *grpc.GrpcCatalogClient) (*Bot, error) {
	tgBot, err := bot.NewBotAPI("8419250904:AAE8hbKcfH8SqY4LQP0blN-d5rRlrt39yC8")
	if err != nil {
		return nil, err
	}
	log := logger.New()

	sessionRepo := mocks.NewSessionRepoMock()
	surveyService, err := usecase.NewSurveyService(sessionRepo, log)
	if err != nil {
		return nil, err
	}
	catalogService := catalog.NewRecommender(catalogClient)

	handler := NewJobHandler(tgBot, surveyService, catalogService, log)
	return &Bot{
		Bot:     tgBot,
		Handler: handler,
		Log:     log,
	}, nil
}
