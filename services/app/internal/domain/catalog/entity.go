package catalog

import (
	"github.com/google/uuid"
)

type ImageItem struct {
	ID            uuid.UUID `json:"id"`
	ObjectID      uuid.UUID `json:"object_id"`
	Gender        string    `json:"gender"`
	CategoryGroup string    `json:"category_group"`
	Category      string    `json:"category"`
	Style         string    `json:"style"`
	Color         string    `json:"color"`
	Season        string    `json:"season"`
	Material      string    `json:"material"`
	Description   string    `json:"description"`
	Ext           string    `json:"ext"`
}

func (itm *ImageItem) Validate() error {
	if itm == nil {
		return nil
	}
	if itm.Category == "" {
		return ErrNoCategory
	}
	if itm.Style == "" {
		return ErrNoStyle
	}
	if itm.Gender == "" {
		return ErrNoGender
	}
	if itm.Color == "" {
		return ErrNoColor
	}
	return nil
}

type IncomingFeature struct {
	Gender   string
	Category string
	Style    string
	Color    string
	Season   string
	Material string
}

type Capsule struct {
	Image []byte // может тут придется поменять, я хз как картинки хранятся
	Items []ImageItem
}
type Recommendations struct {
	Outfits []Outfit
}

type Outfit struct {
	Items []ImageItem
}
