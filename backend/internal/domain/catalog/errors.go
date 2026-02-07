package catalog

import "errors"

var (
	ErrNoCategory        = errors.New("category is required")
	ErrNoStyle           = errors.New("style is required")
	ErrNoGender          = errors.New("gender is required")
	ErrNoColor           = errors.New("color is required")
	ErrNoRecommendations = errors.New("no recommendations")
)
