package grpc

import (
	"capsule-me/internal/domain/catalog"
	capsulegen "capsule-me/internal/gen/capsule-gen"
	common "capsule-me/internal/gen/common"
	lookgen "capsule-me/internal/gen/look-gen"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

type lookResult struct {
	id   int32
	resp *catalog.Recommendations
	err  error
}

func capsuleToDTO(in *catalog.Capsule) (*lookgen.GenerateLooksRequest, error) {
	if in == nil {
		return nil, fmt.Errorf("capsule is nil")
	}

	if len(in.Items) == 0 {
		return nil, fmt.Errorf("capsule has no items")
	}

	items := make([]*capsulegen.Item, 0, len(in.Items))

	for _, item := range in.Items {
		gender := genderToDTO(item.Gender)
		style := styleToDTO(item.Style)
		season := seasonToDTO(item.Season)

		items = append(items, &capsulegen.Item{
			Id:            item.ID,
			Gender:        gender,
			CategoryGroup: item.CategoryGroup,
			Category:      item.Category,
			Style:         style,
			Color:         item.Color,
			Season:        season,
			Material:      item.Material,
			Description:   item.Description,
			Ext:           item.Ext,
		})
	}

	return &lookgen.GenerateLooksRequest{
		Capsule: &capsulegen.Capsule{
			Item: items,
		},
		MaxLooks:      6,
		IncludeImages: true,
		Options: &lookgen.LookGenerationOptions{
			UseMlScoring:        true,
			RequireDiverseItems: true,
			MinItemsPerLook:     3,
			MaxItemsPerLook:     4,
		},
	}, nil
}

func recommendationsToDTO(in *lookgen.LookPack) (*catalog.Recommendations, error) {
	if in == nil {
		return nil, fmt.Errorf("look pack is nil")
	}

	out := &catalog.Recommendations{
		Outfits: make([]catalog.Outfit, 0, len(in.GetLooks())),
	}

	for _, look := range in.GetLooks() {
		if look == nil {
			continue
		}

		outfit := catalog.Outfit{
			Items: make([]catalog.ImageItem, 0, len(look.GetItems())),
		}

		for _, item := range look.GetItems() {
			if item == nil {
				continue
			}

			outfit.Items = append(outfit.Items, catalog.ImageItem{
				ID:            item.GetId(),
				Gender:        genderFromDTO(item.GetGender()),
				CategoryGroup: item.GetCategoryGroup(),
				Category:      item.GetCategory(),
				Style:         styleFromDTO(item.GetStyle()),
				Color:         item.GetColor(),
				Season:        seasonFromDTO(item.GetSeason()),
				Material:      item.GetMaterial(),
				Description:   item.GetDescription(),
				Ext:           item.GetExt(),
			})
		}

		if len(outfit.Items) == 0 {
			continue
		}

		out.Outfits = append(out.Outfits, outfit)
	}

	return out, nil
}

func (cc *GrpcCatalogClient) ForvardRecommendLooks(
	ctx context.Context,
	capsule *catalog.Capsule,
) (*catalog.Recommendations, error) {
	req, err := capsuleToDTO(capsule)
	if err != nil {
		return nil, fmt.Errorf("convert capsule to look request: %w", err)
	}

	req.IncludeImages = true

	stream, err := cc.lookClient.GenerateLooks(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("generate looks: %w", err)
	}

	result := &catalog.Recommendations{
		Outfits: make([]catalog.Outfit, 0),
	}

	imagesByIndex := make(map[int32][]byte)

	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, fmt.Errorf("receive generate looks response: %w", err)
		}

		if resp == nil {
			continue
		}

		if lookPack := resp.GetLookPack(); lookPack != nil {
			recommendations, err := recommendationsToDTO(lookPack)
			if err != nil {
				return nil, fmt.Errorf("convert look pack to recommendations: %w", err)
			}

			result.Outfits = append(result.Outfits, recommendations.Outfits...)
			continue
		}

		if imageChunk := resp.GetImageChunk(); imageChunk != nil {
			imageIndex := imageChunk.GetImageIndex()
			chunk := imageChunk.GetChunk()

			if len(chunk) == 0 {
				continue
			}

			imagesByIndex[imageIndex] = append(imagesByIndex[imageIndex], chunk...)
			continue
		}
	}

	for imageIndex, imageBytes := range imagesByIndex {
		if len(imageBytes) == 0 {
			continue
		}

		if imageIndex < 0 || int(imageIndex) >= len(result.Outfits) {
			continue
		}

		result.Outfits[imageIndex].Image = imageBytes
	}

	for i := range result.Outfits {
		if len(result.Outfits[i].Image) == 0 {
			return nil, fmt.Errorf("look image is empty: index=%d", i)
		}
	}

	return result, nil
}

func (cc *GrpcCatalogClient) RecommendLooks(ctx context.Context, in *catalog.Capsule) (*catalog.Recommendations, error) {
	resChan := make(chan lookResult, cc.workers)

	job := job{
		id:         cc.increment(),
		lookReq:    in,
		ctx:        ctx,
		lookResult: resChan,
		jobtype:    getLooks,
	}

	if ok := cc.submit(job); !ok {
		return nil, errors.New("job не положилась в канал")
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-job.lookResult:
		return res.resp, res.err
	}
}

func genderToDTO(value string) common.Gender {
	switch normalizeEnum(value) {
	case "male", "man", "men":
		return common.Gender_GENDER_MALE
	case "female", "woman", "women":
		return common.Gender_GENDER_FEMALE
	default:
		return common.Gender_GENDER_UNSPECIFIED
	}
}

func styleToDTO(value string) common.Style {
	switch normalizeEnum(value) {
	case "casual":
		return common.Style_STYLE_CASUAL
	case "classic":
		return common.Style_STYLE_CLASSIC
	case "sport", "sports":
		return common.Style_STYLE_SPORT
	default:
		return common.Style_STYLE_UNSPECIFIED
	}
}

func seasonToDTO(value string) common.Season {
	switch normalizeEnum(value) {
	case "winter":
		return common.Season_SEASON_WINTER
	case "spring":
		return common.Season_SEASON_SPRING
	case "summer":
		return common.Season_SEASON_SUMMER
	case "autumn", "fall":
		return common.Season_SEASON_AUTUMN
	default:
		return common.Season_SEASON_UNSPECIFIED
	}
}

func genderFromDTO(value common.Gender) string {
	switch value {
	case common.Gender_GENDER_MALE:
		return "male"
	case common.Gender_GENDER_FEMALE:
		return "female"
	default:
		return ""
	}
}

func styleFromDTO(value common.Style) string {
	switch value {
	case common.Style_STYLE_CASUAL:
		return "casual"
	case common.Style_STYLE_CLASSIC:
		return "classic"
	case common.Style_STYLE_SPORT:
		return "sport"
	default:
		return ""
	}
}

func seasonFromDTO(value common.Season) string {
	switch value {
	case common.Season_SEASON_WINTER:
		return "winter"
	case common.Season_SEASON_SPRING:
		return "spring"
	case common.Season_SEASON_SUMMER:
		return "summer"
	case common.Season_SEASON_AUTUMN:
		return "autumn"
	default:
		return ""
	}
}

func normalizeEnum(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, "-", "_")
	value = strings.ReplaceAll(value, " ", "_")

	return value
}
