package main

import (
    "os"
    "context"
    "log"

    "github.com/jackc/pgx/v5"
    "github.com/go-telegram/bot"
    "bot/internal/storage"
    "bot/internal/handlers"
)

// docker run --name bot-db -e POSTGRES_USER=bot -e POSTGRES_PASSWORD=bot_pass -e POSTGRES_DB=bot_db -p 5432:5432 -d postgres:15

func main() {
    db_config := "postgres://" + os.Getenv("POSTGRES_USER") + ":" + os.Getenv("POSTGRES_PASSWORD") + "@postgres:5432/" + os.Getenv("POSTGRES_DB")

    ctx := context.Background()

    conn, err := pgx.Connect(ctx, db_config)
    if err != nil {
        log.Fatalf("не удалось подключиться к БД: %v", err)
    }
    defer conn.Close(ctx)

    db := storage.New(conn)

    if err := db.InitSchema(ctx); err != nil {
        log.Fatalf("Ошибка инициапизаци: %v", err)
    }

    b, err := bot.New(os.Getenv("BOT_TOKEN"), bot.WithDefaultHandler(handlers.Router(db)))
    if err != nil {
        log.Fatalf("не удалось создать бота: %v", err)
    }

    b.Start(ctx)
}
