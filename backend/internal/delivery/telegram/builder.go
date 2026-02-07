package telegram

import (
	"sync/atomic"

	bot "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var jobSeq uint64

func nextJobID() uint64 {
	return atomic.AddUint64(&jobSeq, 1)
}

func BuildJob(upd bot.Update) (Job, bool) {
	msg := upd.Message
	cq := upd.CallbackQuery

	if cq != nil {
		switch {
		case cq.Message != nil:
			return Job{
				ChatID:          cq.Message.Chat.ID,
				UserID:          cq.From.ID,
				Type:            JobAnswer,
				Data:            cq.Data,
				MessageID:       &cq.Message.MessageID,
				UpdateID:        upd.UpdateID,
				CallbackQueryID: &cq.ID,
				JobID:           nextJobID(),
			}, true
		default:
			return Job{
				ChatID:          0,
				UserID:          cq.From.ID,
				Type:            JobAnswer,
				Data:            cq.Data,
				UpdateID:        upd.UpdateID,
				CallbackQueryID: &cq.ID,
				JobID:           nextJobID(),
			}, true
		}

	}

	if msg != nil && msg.IsCommand() {
		switch msg.Command() {
		case "start":
			return Job{
				ChatID:   upd.Message.Chat.ID,
				UserID:   upd.Message.From.ID,
				Type:     JobStart,
				Data:     "start",
				UpdateID: upd.UpdateID,
				JobID:    nextJobID(),
			}, true
		default:
			return Job{}, false
		}
	}
	return Job{}, false
}
