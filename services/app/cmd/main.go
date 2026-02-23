package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"capsule-me/internal/delivery/telegram"
	"capsule-me/internal/runtime/workerpool"

	bot "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// db, err := postgres.NewPostgres(ctx)
	// if err != nil {
	// 	log.Fatalf("failed postgres:%v", err)
	// }
	// defer db.Close()

	b, err := telegram.NewBot()
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

	router(ctx, b.Bot, updates, pool)
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
