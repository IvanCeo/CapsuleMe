package catalog

import "context"

type Recommender struct {
	repo ImageRepository
}

type ImageRepository interface {
	Recommend(ctx context.Context, f *IncomingFeature) (*Capsule, error)
	RecommendLooks(ctx context.Context, in *Capsule) (*Recommendations, error)
}

func NewRecommender(repo ImageRepository) *Recommender {
	return &Recommender{repo: repo}
}

func (r *Recommender) Recommend(ctx context.Context, f *IncomingFeature) (*Capsule, error) {
	return r.repo.Recommend(ctx, f)
}

func (r *Recommender) RecommendLooks(ctx context.Context, in *Capsule) (*Recommendations, error) {
	return r.repo.RecommendLooks(ctx, in)
}
