package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const searchEndpointPath = "/semantic-geo-search/"

func ForwardSearchRequest(ctx context.Context, payload map[string]any) (int, []byte, error) {
	expandedQuery := ""
	if expand, ok := payload["expand"].(bool); ok && expand {
		query, _ := payload["query"].(string)
		expandedQuery = strings.TrimSpace(query)
		if nextQuery, err := ExpandQuery(query); err == nil && strings.TrimSpace(nextQuery) != "" {
			expandedQuery = nextQuery
		}
		payload["query"] = expandedQuery
	}
	delete(payload, "expand")

	body, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to marshal search payload: %w", err)
	}

	targetURL := mainBackendURL() + searchEndpointPath

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return 0, nil, fmt.Errorf("failed to build search request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := indexerHTTPClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if expandedQuery != "" && json.Valid(respBody) {
		wrapped, err := json.Marshal(struct {
			ExpandedQuery string          `json:"expanded_query"`
			Results       json.RawMessage `json:"results"`
		}{ExpandedQuery: expandedQuery, Results: respBody})
		if err == nil {
			return resp.StatusCode, wrapped, nil
		}
	}
	return resp.StatusCode, respBody, nil
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
	if rawExpand, ok := payload["expand"]; ok {
		if _, ok := rawExpand.(bool); !ok {
			return nil, fmt.Errorf("expand must be a boolean")
		}
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
