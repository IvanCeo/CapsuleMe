package survey

import (
	"time"

	"github.com/google/uuid"
)

type SurveyEngine struct{}

func (e *SurveyEngine) StartSession(def *SurveyDefinition, userID int64) (*SurveySession, error) {
	err := def.Validate()
	if err != nil {
		return nil, err
	}
	return &SurveySession{
		ID:                uuid.New(),
		UserID:            userID,
		SurveyID:          def.ID,
		CurrentQuestionID: def.Questions[0].ID,
	}, nil
}

func (e *SurveyEngine) AnswerQuestion(def *SurveyDefinition, s *SurveySession, answerValue string) error {
	if e.IsDone(s) {
		return ErrSurveyCompleted
	}
	if s.HasAnswer(s.CurrentQuestionID) {
		return ErrAlreadyAnswered
	}
	if err := def.IsQuestionExists(s.CurrentQuestionID); err != nil {
		return err
	}
	if err := def.IsAnswerExists(s.CurrentQuestionID, answerValue); err != nil {
		return err
	}
	s.Answers = append(s.Answers, UserAnswer{QuestionID: s.CurrentQuestionID, Value: answerValue, Timestamp: time.Now()})
	next := e.findNext(def, s)

	s.CurrentQuestionID = next
	if next == "" {
		s.IsDone = true
	}

	return nil
}

func (e *SurveyEngine) GetCurrentQuestion(def *SurveyDefinition, s *SurveySession) (*Question, error) {
	for _, q := range def.Questions {
		if q.ID == s.CurrentQuestionID {
			return &q, nil
		}
	}
	return nil, ErrUnknownQuestion
}

func (e *SurveyEngine) findNext(def *SurveyDefinition, s *SurveySession) string {
	if len(def.Questions) == 0 {
		return ""
	}
	for _, q := range def.Questions {
		if !s.HasAnswer(q.ID) {
			return q.ID
		}
	}
	return ""
}

func (e *SurveyEngine) NextQuestion(def *SurveyDefinition, s *SurveySession) *Question {
	for _, q := range def.Questions {
		if !s.HasAnswer(q.ID) {
			return &q
		}
	}
	return nil
}

func (e *SurveyEngine) IsDone(s *SurveySession) bool {
	return s.IsDone
}
