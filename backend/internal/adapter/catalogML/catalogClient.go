package adapter

import (
	"bytes"
	"capsule-me/internal/domain/catalog"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

type CatalogClient struct {
	baseURL    string
	httpClient *http.Client
}

type RecommendRequest struct {
	ChatID  int64   `json:"chat_id"`
	Gender  string  `json:"gender"`
	Style   string  `json:"style"`
	Season  string  `json:"season"`
	Palette *string `json:"palette,omitempty"`
}

type RecommendResponse struct {
	OK     bool                   `json:"ok"`
	Result map[string]interface{} `json:"result,omitempty"`
	Detail any                    `json:"detail,omitempty"`
}

func NewClient() *http.Client {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     80 * time.Second,

		DialContext: (&net.Dialer{
			Timeout:   2 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,

		TLSHandshakeTimeout:   2 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
	}
}

func NewCatalogClient(baseURL string) *CatalogClient {
	h := NewClient()
	return &CatalogClient{
		httpClient: h,
		baseURL:    baseURL,
	}
}

func toRecommendRequest(f *catalog.IncomingFeature, chatID int64) (*RecommendRequest, error) {
	if f == nil {
		return nil, errors.New("feature is nil")
	}

	var palette *string
	v := strings.ToLower(strings.TrimSpace(f.Color))

	switch v {
	case "", "none":
		palette = nil
	case "dark", "light", "bright":
		palette = &v
	default:
		return nil, fmt.Errorf("invalid palette: %q", f.Color)
	}

	season := strings.ToLower(strings.TrimSpace(f.Season))
	if season == "" || season == "all-seasons" {
		return nil, fmt.Errorf("invalid season: %q", f.Season)
	}

	gender := strings.ToLower(strings.TrimSpace(f.Gender))
	style := strings.ToLower(strings.TrimSpace(f.Style))

	return &RecommendRequest{
		ChatID:  chatID,
		Gender:  gender,
		Style:   style,
		Season:  season,
		Palette: palette,
	}, nil
}

func (c *CatalogClient) Recommend(ctx context.Context, f *catalog.IncomingFeature, chatID int64) error {
	reqDTO, err := toRecommendRequest(f, chatID)
	if err != nil {
		return err
	}

	body, err := json.Marshal(reqDTO)
	if err != nil {
		return fmt.Errorf("marshal recommend request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("post to outfit service: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var decoded RecommendResponse
	_ = json.Unmarshal(respBody, &decoded)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if len(respBody) > 0 {
			return fmt.Errorf("outfit service status=%d body=%s", resp.StatusCode, string(respBody))
		}
		return fmt.Errorf("outfit service status=%d", resp.StatusCode)
	}

	if decoded.OK == false && len(respBody) > 0 {
		return fmt.Errorf("outfit service returned ok=false body=%s", string(respBody))
	}

	return nil
}
