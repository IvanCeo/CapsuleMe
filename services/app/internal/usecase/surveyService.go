package usecase

import (
	"capsule-me/internal/domain/catalog"
	"capsule-me/internal/domain/survey"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
)

// оркестрирует работу доменных сервисов
type SurveyService struct {
	engine   *survey.SurveyEngine
	def      *survey.SurveyDefinition
	mapper   *survey.FeatureMapper
	session  SessionRepo
	log      *slog.Logger
	cache    cache
	feedback postgres
}

type postgres interface {
	SaveFeedback(ctx context.Context, key, value string) error
}

type cache interface {
	SaveToCache(ctx context.Context, key string, value interface{}) error
	GetFromCache(ctx context.Context, key string) (string, error)
}

type SessionRepo interface {
	Save(*survey.SurveySession) error
	GetByUser(userID int64) (*survey.SurveySession, error)
	DeleteByUser(userID int64)
}

func NewSurveyService(cache cache, repo SessionRepo, postgres postgres, log *slog.Logger) (*SurveyService, error) {
	root, _ := os.Getwd()
	def, err := survey.LoadSurveyDefinitionYAML(filepath.Join(root, "configs", "surveyDefinition.yaml"))
	if err != nil {
		wd, _ := os.Getwd()
		fmt.Println("working dir:", wd)
		return nil, err
	}

	cfg, err := survey.LoadMappingConfigYAML(filepath.Join(root, "configs", "featureMapping.yaml"))
	if err != nil {
		return nil, err
	}

	return &SurveyService{
		engine:   &survey.SurveyEngine{},
		def:      def,
		mapper:   &survey.FeatureMapper{Config: cfg},
		session:  repo,
		log:      log,
		cache:    cache,
		feedback: postgres,
	}, nil
}

func (ser *SurveyService) StartSurvey(userID int64) (*survey.Question, error) {

	session, err := ser.engine.StartSession(ser.def, userID)
	if err != nil {
		ser.log.Error(
			"failed to start session",
			"userID", userID,
			"err", err,
		)
		return nil, err
	}

	err = ser.session.Save(session)
	if err != nil {
		ser.log.Error(
			"failed to save session",
			"userID", userID,
			"err", err,
		)
		return nil, err
	}
	q, err := ser.engine.GetCurrentQuestion(ser.def, session)
	if err != nil {
		ser.log.Error(
			"failed to get current question",
			"userID", userID,
			"err", err,
		)
		return nil, err
	}

	return q, nil
}

func (ser *SurveyService) GetSessionByUser(userID int64) (*survey.SurveySession, error) {
	s, err := ser.session.GetByUser(userID)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (ser *SurveyService) IsDone(userID int64) (bool, error) {
	s, err := ser.session.GetByUser(userID)
	if err != nil {
		return true, err
	}
	return ser.engine.IsDone(s), nil
}

func (ser *SurveyService) AnswerQuestion(userID int64, answerValue string) error {
	session, err := ser.GetSessionByUser(userID)
	if err != nil {
		return err
	}

	return ser.engine.AnswerQuestion(ser.def, session, answerValue)
}

func (ser *SurveyService) GetCurrentQuestion(userID int64) (*survey.Question, error) {
	session, err := ser.GetSessionByUser(userID)
	if err != nil {
		return nil, err
	}

	q, err := ser.engine.GetCurrentQuestion(ser.def, session)
	if err != nil {
		return nil, err
	}

	return q, nil
}

func (ser *SurveyService) MapIncomingFeature(session *survey.SurveySession) (*catalog.IncomingFeature, error) {
	f, err := ser.mapper.MapAnswers(session)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (ser *SurveyService) SaveToCache(ctx context.Context, jobID uint64, capsule []byte) error {
	return ser.cache.SaveToCache(ctx, strconv.FormatUint(jobID, 10), capsule) // расплата за uint
}

func (ser *SurveyService) GetFromCache(ctx context.Context, key string) (string, error) {
	return ser.cache.GetFromCache(ctx, key)
}

func (ser *SurveyService) SaveFeedback(ctx context.Context, key, value string) error {
	return ser.feedback.SaveFeedback(ctx, key, value)
}
