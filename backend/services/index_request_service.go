package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const (
	defaultMainBackendURL = "http://localhost:8080"
	indexEndpointPath     = "/api/v1/semantic-geo-search/index"
)

// IndexRequest matches the payload sent by the frontend for indexing.
type IndexRequest struct {
	S3Path   string  `json:"s3_path" binding:"required"`
	Region   string  `json:"region"`
	BBoxMinX float64 `json:"bbox_min_x" binding:"required"`
	BBoxMaxX float64 `json:"bbox_max_x" binding:"required"`
	BBoxMinY float64 `json:"bbox_min_y" binding:"required"`
	BBoxMaxY float64 `json:"bbox_max_y" binding:"required"`
	All      bool    `json:"all"`
	Count    int64   `json:"count,omitempty"`
}

func SendIndexRequest(ctx context.Context, payload IndexRequest) error {
	targetURL := mainBackendURL() + indexEndpointPath

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal index request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to build index request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := indexerHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("index request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("main backend returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
}

func mainBackendURL() string {
	base := strings.TrimSpace(os.Getenv("MAIN_BACKEND_URL"))
	if base == "" {
		base = defaultMainBackendURL
	}
	return strings.TrimRight(base, "/")
}
