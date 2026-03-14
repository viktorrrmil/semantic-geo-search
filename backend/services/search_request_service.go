package services

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const searchEndpointPath = "/semantic-geo-search/"

func ForwardSearchRequest(ctx context.Context, payload []byte) (int, []byte, error) {
	targetURL := mainBackendURL() + searchEndpointPath

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(payload))
	if err != nil {
		return 0, nil, fmt.Errorf("failed to build search request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := indexerHTTPClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body, nil
}

func NormalizeSearchPayload(payload map[string]any) (map[string]any, error) {
	rawQuery, ok := payload["query"]
	if !ok {
		return nil, fmt.Errorf("query is required")
	}
	query, ok := rawQuery.(string)
	if !ok || strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query must be a non-empty string")
	}

	if top, ok := payload["top_k"]; ok && !isJSONNumber(top) {
		return nil, fmt.Errorf("top_k must be a number")
	}
	if count, ok := payload["count"]; ok && !isJSONNumber(count) {
		return nil, fmt.Errorf("count must be a number")
	}

	return payload, nil
}

func isJSONNumber(v any) bool {
	switch v.(type) {
	case float64, float32, int, int32, int64, uint, uint32, uint64:
		return true
	default:
		return false
	}
}
