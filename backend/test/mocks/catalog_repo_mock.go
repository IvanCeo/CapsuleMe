package mocks

import (
	"capsule-me/internal/domain/catalog"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

type MockImageRepository struct {
	Items []*catalog.ImageItem
	Err   error
}

func (m *MockImageRepository) GetItemByFeatures(features *catalog.IncomingFeature) ([]*catalog.ImageItem, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.Items, nil
}

type MockRepo struct{}

func (m *MockRepo) GetItemByFeatures(features *catalog.IncomingFeature) ([]*catalog.ImageItem, error) {
	return []*catalog.ImageItem{
		{Category: "sport", Color: "white"},
		{Style: "casual", Season: "winter"},
		{Gender: "men", Material: "cotton"},
		{Gender: "woman", Material: "synthetic"},
	}, nil
}

type Repository struct {
	csvPath string
}

func NewRepository(csvPath string) *Repository {
	return &Repository{csvPath: csvPath}
}

// GetItemByFeatures читает data/metadata_uuid.csv и фильтрует под входные фичи.
func (r *Repository) GetItemByFeatures(f *catalog.IncomingFeature) ([]*catalog.ImageItem, error) {
	if f == nil {
		return nil, fmt.Errorf("features is nil")
	}

	file, err := os.Open(filepath.Clean(r.csvPath))
	if err != nil {
		return nil, fmt.Errorf("open csv: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1 // допускаем переменное число столбцов

	// Заголовок
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	// Индексы колонок
	colIndex := map[string]int{}
	for i, h := range header {
		colIndex[strings.ToLower(strings.TrimSpace(h))] = i
	}

	// Безопасное получение колонки
	get := func(row []string, col string) string {
		idx, ok := colIndex[col]
		if !ok || idx < 0 || idx >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[idx])
	}

	// Нормализуем входные фичи
	inGender := lower(f.Gender)
	inStyle := lower(f.Style)
	inSeason := lower(f.Season)
	inCategory := lower(f.Category)
	inColor := lower(f.Color)
	inMaterial := lower(f.Material)

	allowedSeasons := allowedSeasonsFor(inSeason)

	out := make([]*catalog.ImageItem, 0, 64)

	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read row: %w", err)
		}

		// поля CSV
		uuidStr := get(row, "uuid")
		ext := lower(get(row, "ext"))

		// только jpg/jpeg, чтобы minio mock всегда находил <uuid>.jpg
		if ext != "" && ext != "jpg" && ext != "jpeg" {
			continue
		}

		// UUID обязателен
		id, err := uuid.Parse(uuidStr)
		if err != nil {
			continue
		}

		gender := lower(get(row, "gender"))
		category := lower(get(row, "category"))
		style := lower(get(row, "style"))
		color := lower(get(row, "color"))
		season := lower(get(row, "season"))
		material := lower(get(row, "material"))
		description := get(row, "description")

		// фильтрация
		// gender: совпадает или unisex
		if inGender != "" {
			if gender != inGender && gender != "unisex" {
				continue
			}
		}

		// style: если задан — точное совпадение
		if inStyle != "" && style != inStyle {
			continue
		}

		// season: с алиасами как в python recommend_outfit
		if inSeason != "" {
			if season == "" {
				season = "all-seasons"
			}
			if !contains(allowedSeasons, season) {
				continue
			}
		}

		// category: если задана
		if inCategory != "" && category != inCategory {
			continue
		}

		// color: если задан
		if inColor != "" && color != inColor {
			continue
		}

		// material: если задан
		if inMaterial != "" && material != inMaterial {
			continue
		}

		item := &catalog.ImageItem{
			ID:          id,
			ObjectID:    id, // можно использовать как key для S3/MinIO
			Gender:      gender,
			Category:    category,
			Style:       style,
			Color:       color,
			Season:      season,
			Material:    material,
			Description: description,
		}

		out = append(out, item)
	}

	return out, nil
}

// allowedSeasonsFor реализует сезонный маппинг как в python:
func allowedSeasonsFor(season string) []string {
	switch lower(season) {
	case "winter":
		return []string{"winter", "autumn-winter", "all-seasons"}
	case "autumn", "fall":
		return []string{"autumn", "autumn-winter", "all-seasons"}
	case "spring":
		return []string{"spring", "spring-summer", "all-seasons"}
	case "summer":
		return []string{"summer", "spring-summer", "all-seasons"}
	default:
		return []string{"all-seasons"}
	}
}

func lower(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func contains(arr []string, v string) bool {
	for _, x := range arr {
		if x == v {
			return true
		}
	}
	return false
}
