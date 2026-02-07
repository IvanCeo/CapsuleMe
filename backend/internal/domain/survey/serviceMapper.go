package survey

import (
	"capsule-me/internal/domain/catalog"
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// FeatureMapping описывает связь вопроса с полем фич
// то есть то, как сервис интерпритирует ответы
type FeatureMapping struct {
	QuestionID string            `yaml:"question_id" json:"question_id"`
	Target     string            `yaml:"target" json:"target"`
	ValueMap   map[string]string `yaml:"value_map,omitempty" json:"value_map,omitempty"`
	Default    string            `yaml:"default,omitempty" json:"default,omitempty"`
	Required   bool              `yaml:"required,omitempty" json:"required,omitempty"`
}

// MappingConfig — набор правил для опроса
type MappingConfig struct {
	SurveyID string           `yaml:"survey_id" json:"survey_id"`
	Version  string           `yaml:"version" json:"version"`
	Rules    []FeatureMapping `yaml:"rules" json:"rules"`
}

// FeatureMapper — доменный сервис
type FeatureMapper struct {
	Config *MappingConfig
}

// MapAnswers — строит IncomingFeature из ответов пользователя
func (m *FeatureMapper) MapAnswers(s *SurveySession) (catalog.IncomingFeature, error) {
	if s == nil {
		return catalog.IncomingFeature{}, errors.New("session is nil")
	}
	if m.Config == nil {
		return catalog.IncomingFeature{}, ErrEmptyMapConfig
	}

	ans := make(map[string]string)
	for _, a := range s.Answers {
		ans[a.QuestionID] = a.Value
	}

	var f catalog.IncomingFeature

	for _, rule := range m.Config.Rules {
		val := ans[rule.QuestionID]
		if val == "" {
			if rule.Default != "" {
				val = rule.Default
			} else if rule.Required {
				return catalog.IncomingFeature{}, errors.New("missing required answer: " + rule.QuestionID)
			} else {
				continue
			}
		}
		if rule.ValueMap != nil {
			if mapped, ok := rule.ValueMap[val]; ok {
				val = mapped
			}
		}

		switch rule.Target {
		case "Gender":
			f.Gender = val
		case "Category":
			f.Category = val
		case "Style":
			f.Style = val
		case "Color":
			f.Color = val
		case "Season":
			f.Season = val
		case "Material":
			f.Material = val
		}
	}

	return f, nil
}

func LoadMappingConfigYAML(path string) (*MappingConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("LoadMappingConfigYAML error: %v", err)
	}
	var cfg MappingConfig
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return nil, fmt.Errorf("yaml.Unmarshal error: %v", err)
	}
	return &cfg, nil
}
