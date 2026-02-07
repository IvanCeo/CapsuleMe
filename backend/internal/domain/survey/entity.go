package survey

import (
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

type Question struct {
	ID      string
	Text    string
	Options []AnswerOption
}

type AnswerOption struct {
	Value string
	Label string
}

// данная структура описывает сценарий опроса - какие есть вопросы и варианты ответов
// это "что спрашиваем у пользователя"
type SurveyDefinition struct {
	ID        string
	Title     string
	Questions []Question
	Version   string
}

func LoadSurveyDefinitionYAML(path string) (*SurveyDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("LoadSurveyDefinitionYAML error: %v", err)
	}
	var def SurveyDefinition
	err = yaml.Unmarshal(data, &def)
	if err != nil {
		return nil, fmt.Errorf("yaml.Unmarshal error: %v", err)
	}
	return &def, nil
}

func (s *SurveyDefinition) Validate() error {
	if len(s.Questions) == 0 {
		return ErrNoQuestions
	}
	return nil
}

func (s *SurveyDefinition) IsQuestionExists(id string) error {
	for _, q := range s.Questions {
		if q.ID == id {
			return nil
		}
	}
	return ErrUnknownQuestion
}

func (s *SurveyDefinition) IsAnswerExists(questionID, answerValue string) error {
	for _, q := range s.Questions {
		if q.ID == questionID {
			for _, a := range q.Options {
				if a.Value == answerValue {
					return nil
				}
			}
		}
	}
	return ErrInvalidAnswer
}

type SurveySession struct {
	ID                uuid.UUID
	SurveyID          string
	UserID            int64
	CurrentQuestionID string // какой вопрос сейчас question.id
	Answers           []UserAnswer
	IsDone            bool
}

func (s *SurveySession) HasAnswer(questionID string) bool {
	var collection []string
	for _, a := range s.Answers {
		collection = append(collection, a.QuestionID)
	}

	for _, sid := range collection {
		if sid == questionID {
			return true
		}
	}
	return false
}

type UserAnswer struct {
	QuestionID string
	Value      string
	Timestamp  time.Time
}
