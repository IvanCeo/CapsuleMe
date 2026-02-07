package catalog

import "context"

type Recommender struct {
	repo ImageRepository
}

type ImageRepository interface {
	Recommend(ctx context.Context, f *IncomingFeature, chatID int64) error
}

func NewRecommender(repo ImageRepository) *Recommender {
	return &Recommender{repo: repo}
}

func (r *Recommender) Recommend(ctx context.Context, f *IncomingFeature, chatID int64) error {
	return r.repo.Recommend(ctx, f, chatID)
}
