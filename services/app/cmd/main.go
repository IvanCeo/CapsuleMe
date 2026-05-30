package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"capsule-me/internal/delivery/grpc"
	"capsule-me/internal/delivery/telegram"
	"capsule-me/internal/domain/catalog"
	"capsule-me/internal/logger"
	"capsule-me/internal/runtime/workerpool"

	bot "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
)

// имитация абстракции сервиса реккомендации
type cargo struct {
	catalog *grpc.GrpcCatalogClient
	logger  *slog.Logger
}

func newCargo(c *grpc.GrpcCatalogClient, l *slog.Logger) *cargo {
	return &cargo{catalog: c, logger: l}
}

func (c *cargo) cargoHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := c.catalog.ForvardRecommend(context.Background(), &catalog.IncomingFeature{}); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		c.logger.Error("maintest err", "err", err)
		return
	}
	w.WriteHeader(http.StatusOK)
	c.logger.Info("maintest success")
}

func router(
	ctx context.Context,
	tg *bot.BotAPI,
	updates <-chan bot.Update,
	pool *workerpool.WorkerPool,
) {
	for {
		select {
		case <-ctx.Done():
			tg.StopReceivingUpdates()

			graceCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			<-graceCtx.Done()

			log.Println("bot stopped")
			return

		case upd, ok := <-updates:
			if !ok {
				log.Println("updates channel closed")
				return
			}

			job, ok := telegram.BuildJob(upd)
			if !ok {
				continue
			}

			if ok := pool.Submit(job); !ok {

				telegram.ReplyBusy(tg, job)
			}
		}
	}
}

//$env:HTTPS_PROXY="http://127.0.0.1:10808"
//$env:HTTP_PROXY="http://127.0.0.1:10808"

func main() {
	log2 := logger.New()

	godotenv.Load("app.env")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client, err := grpc.NewGrpcCatalogClient(5, log2, "localhost:50052", "localhost:50053")
	// localhost:50052 capsuleService
	// localhost:50053 lookService
	if err != nil {
		log.Fatalf("failed to create gRPC client: %v", err)
	}

	client.Start()
	defer client.Stop()

	b, err := telegram.NewBot(client)
	if err != nil {
		log.Fatalf("failed to init bot: %v", err)
	}

	updates := b.Bot.GetUpdatesChan(telegram.NewUpdateConfig())

	pool := workerpool.NewWorkerPool(
		5,
		100,
		b.Log,
	)
	pool.Start(ctx, b.Handler.Handle)

	go router(ctx, b.Bot, updates, pool)

	// для замеров
	cargo := newCargo(client, log2)

	mux := http.NewServeMux()
	mux.HandleFunc("/cargo", cargo.cargoHandler)

	srv := &http.Server{
		Addr:           ":5052",
		Handler:        mux,
		ReadTimeout:    5 * time.Second,
		WriteTimeout:   5 * time.Second,
		IdleTimeout:    30 * time.Second,
		MaxHeaderBytes: 1 << 10,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log2.Error("HTTP server error", "err", err)
		}
	}()

	<-ctx.Done()
	log2.Info("gracefull shutdown...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log2.Error("HTTP shutdown error", "err", err)
	}

	log2.Info("сервер остановлен")

}
