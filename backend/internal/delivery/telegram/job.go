package telegram

type JobType int

const (
	JobStart JobType = iota
	JobAnswer
)

type Job struct {
	ChatID          int64
	UserID          int64
	Type            JobType
	Data            string
	MessageID       *int
	UpdateID        int
	CallbackQueryID *string
	JobID           uint64
}
