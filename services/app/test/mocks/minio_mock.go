//go:build ignore

package mocks

import (
	"capsule-me/internal/domain/catalog"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type MinioMock struct {
	source string
}

func NewMinioMock(path string) *MinioMock {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return &MinioMock{source: abs}
}

func (m *MinioMock) GetImage(item catalog.ImageItem) (*os.File, error) {
	id := item.ID.String()

	ext := strings.TrimSpace(item.Ext)
	if ext == "" {
		ext = "jpg" // fallback
	}
	ext = strings.TrimPrefix(ext, ".")

	path := filepath.Clean(filepath.Join(m.source, id+"."+ext))
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file not found: %s", path)
		}
		return nil, err
	}
	return f, nil
}
