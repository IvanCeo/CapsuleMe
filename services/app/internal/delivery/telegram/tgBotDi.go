package telegram

import (
	"capsule-me/internal/delivery/grpc"
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

// type ImageRepository interface {
// 	Recommend(ctx context.Context, f *IncomingFeature, chatID int64) error
// } этот интерфейс должна исполнять иньекция в catalogservice

func NewBot() (*Bot, error) {
	bot, err := bot.NewBotAPI(os.Getenv("TG_TOKEN"))
	if err != nil {
		return nil, err
	}
	log := logger.New()

	// url для grpc сервиса
	catalogClient, err := grpc.NewGrpcCatalogClient(5, "djsk", log)
	if err != nil {
		return nil, err
	}
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
