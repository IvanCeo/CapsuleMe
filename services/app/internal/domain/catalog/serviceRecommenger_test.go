//go:build ignore

package catalog_test

import (
	"capsule-me/internal/domain/catalog"
	"capsule-me/test/mocks"
	"testing"
)

func TestGenerateOutfits_Basic(t *testing.T) {
	items := []*catalog.ImageItem{
		{Category: "t-shirt"},
		{Category: "pants"},
		{Category: "shoes"},
	}

	rec := catalog.NewRecommender(nil)
	result, err := rec.GenerateOutfits(items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) == 0 {
		t.Errorf("expected non-empty result")
	}
}

// func (r *Recommender) GetItemByFeatures(ctx context.Context, feature *IncomingFeature) ([]*ImageItem, error) {
func TestGetItemByFeatures_Basic(t *testing.T) {
	rec := catalog.NewRecommender(&mocks.MockRepo{})
	items, err := rec.GetItemByFeatures(&catalog.IncomingFeature{
		Gender:   "women",
		Category: "sport",
		Style:    "Classic",
		Color:    "black",
		Season:   "winter",
		Material: "cotton",
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(items) == 0 {
		t.Errorf("expected at least 1 item")
	}
}
