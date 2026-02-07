package telegram

import bot "github.com/go-telegram-bot-api/telegram-bot-api/v5"

func ReplyBusy(tg *bot.BotAPI, job Job) {
	if job.CallbackQueryID != nil && *job.CallbackQueryID != "" {
		_, _ = tg.Request(bot.NewCallback(*job.CallbackQueryID, "Сейчас много запросов, попробуй чуть позже"))
	}

	if job.ChatID != 0 {
		_, _ = tg.Send(bot.NewMessage(job.ChatID, "Сейчас много запросов, попробуй чуть позже"))
	}
}

func NewUpdateConfig() bot.UpdateConfig {
	return bot.UpdateConfig{
		Offset:  0,
		Limit:   100,
		Timeout: 30,
	}
}
