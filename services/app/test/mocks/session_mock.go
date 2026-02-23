package mocks

import (
	"capsule-me/internal/domain/survey"
	"errors"
	"sync"
)

var ErrNoSessionForUser = errors.New("no sessions for user")

type SessionRepoMock struct {
	mu       sync.RWMutex
	byUserID map[int64]*survey.SurveySession
}

func NewSessionRepoMock() *SessionRepoMock {
	return &SessionRepoMock{
		byUserID: make(map[int64]*survey.SurveySession),
	}
}

func (s *SessionRepoMock) Save(session *survey.SurveySession) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byUserID[session.UserID] = session
	return nil
}

func (s *SessionRepoMock) GetByUser(userID int64) (*survey.SurveySession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.byUserID[userID]
	if !ok {
		return nil, ErrNoSessionForUser
	}
	return session, nil
}

func (s *SessionRepoMock) DeleteByUser(userID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.byUserID, userID)
}
