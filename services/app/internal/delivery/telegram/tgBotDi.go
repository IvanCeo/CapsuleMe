package telegram

import (
	p "capsule-me/internal/adapter/postgres"
	"capsule-me/internal/delivery/grpc"
	"capsule-me/internal/domain/catalog"
	"capsule-me/internal/logger"
	"capsule-me/internal/usecase"
	"capsule-me/test/mocks"
	"context"
	"errors"
	"log/slog"
	"os"

	bot "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Bot struct {
	Bot     *bot.BotAPI
	Handler *JobHandler
	Log     *slog.Logger
}

func NewBot(catalogClient *grpc.GrpcCatalogClient) (*Bot, error) {
	TG_TOKEN := os.Getenv("TG_TOKEN")
	if TG_TOKEN == "" {
		return nil, errors.New("no tg token in env")
	}
	tgBot, err := bot.NewBotAPI(TG_TOKEN)
	if err != nil {
		return nil, err
	}
	log := logger.New()

	// REDIS_ADDR := os.Getenv("REDIS_ADDR")
	// REDIS_PASS := os.Getenv("REDIS_PASS")
	// if REDIS_ADDR == "" || REDIS_PASS == "" {
	// 	return nil, errors.New("no redis env")
	// }
	// cache, err := redis.NewRedis(REDIS_ADDR, REDIS_PASS)
	// if err != nil {
	// 	return nil, err
	// }

	cache := mocks.NewCache(3)

	ctx := context.Background()
	postgres, err := p.NewPostgres(ctx)
	if err != nil {
		return nil, err
	}
	cfg := p.LoadConfigFromEnv()

	err = postgres.Up(ctx, cfg)
	if err != nil {
		return nil, err
	}

	// if REDIS_ADDR == "" || REDIS_PASS == "" {
	// 	return nil, errors.New("no migrate")
	// }

	sessionRepo := mocks.NewSessionRepoMock()

	surveyService, err := usecase.NewSurveyService(cache, sessionRepo, postgres, log)
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
