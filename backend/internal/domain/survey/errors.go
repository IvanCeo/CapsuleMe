package survey

import "errors"

var (
	ErrUnknownQuestion = errors.New("unknown question id")
	ErrInvalidAnswer   = errors.New("invalid answer value")
	ErrAlreadyAnswered = errors.New("question already answered")
	ErrSurveyCompleted = errors.New("survey already completed")
	ErrNoQuestions     = errors.New("no questions in survey")

	ErrEmptyMapConfig = errors.New("mapping config is nil")
)
