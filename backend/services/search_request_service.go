package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
)

const searchEndpointPath = "/semantic-geo-search/"

type searchRequestParams struct {
	Query         string
	TopK          int
	Expand        bool
	Hybrid        bool
	Category      string
	Alpha         float64
	Beta          float64
	DecayRadiusKm float64
	CenterLat     *float64
	CenterLng     *float64
}

type upstreamSearchEnvelope struct {
	ExpandedQuery string          `json:"expanded_query"`
	Results       json.RawMessage `json:"results"`
	Rows          json.RawMessage `json:"rows"`
}

func ForwardSearchRequest(ctx context.Context, payload map[string]any) (int, []byte, error) {
	params, err := parseSearchRequestParams(payload)
	if err != nil {
		return 0, nil, err
	}

	query := params.Query
	expandedQuery := ""
	if params.Expand {
		expandedQuery = strings.TrimSpace(query)
		if nextQuery, err := ExpandQuery(query); err == nil && strings.TrimSpace(nextQuery) != "" {
			expandedQuery = nextQuery
		}
		query = expandedQuery
	}

	forwardTopK := params.TopK
	if params.Hybrid {
		forwardTopK = maxInt(20, params.TopK*3)
	}

	upstreamPayload := map[string]any{
		"query": query,
		"count": forwardTopK,
		"top_k": forwardTopK,
	}

	body, err := json.Marshal(upstreamPayload)
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
	if !params.Hybrid {
		if params.Expand && expandedQuery != "" && json.Valid(respBody) {
			if wrapped, err := json.Marshal(struct {
				ExpandedQuery string          `json:"expanded_query"`
				Results       json.RawMessage `json:"results"`
			}{ExpandedQuery: expandedQuery, Results: respBody}); err == nil {
				return resp.StatusCode, wrapped, nil
			}
		}
		return resp.StatusCode, respBody, nil
	}

	results, parsedExpandedQuery, ok := decodeSearchResults(respBody)
	if !ok {
		return resp.StatusCode, respBody, nil
	}

	ranked := RerankResults(results, HybridRankingParams{
		Alpha:         params.Alpha,
		Beta:          params.Beta,
		DecayRadiusKm: params.DecayRadiusKm,
		CenterLat:     params.CenterLat,
		CenterLng:     params.CenterLng,
		Category:      params.Category,
	})

	trimmed := ranked
	if len(trimmed) > params.TopK {
		trimmed = trimmed[:params.TopK]
	}

	if params.Expand && expandedQuery == "" {
		expandedQuery = parsedExpandedQuery
	}

	wrapped, err := marshalRankedResults(trimmed, params.Expand && expandedQuery != "", expandedQuery)
	if err != nil {
		return resp.StatusCode, respBody, nil
	}
	return resp.StatusCode, wrapped, nil
}

func NormalizeSearchPayload(payload map[string]any) (map[string]any, error) {
	if _, err := parseSearchRequestParams(payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func parseSearchRequestParams(payload map[string]any) (searchRequestParams, error) {
	rawQuery, ok := payload["query"]
	if !ok {
		return searchRequestParams{}, fmt.Errorf("query is required")
	}
	query, ok := rawQuery.(string)
	if !ok || strings.TrimSpace(query) == "" {
		return searchRequestParams{}, fmt.Errorf("query must be a non-empty string")
	}

	topK := defaultSearchTopK(payload)
	if value, exists := payload["top_k"]; exists {
		parsed, err := intFromJSONNumber(value)
		if err != nil {
			return searchRequestParams{}, fmt.Errorf("top_k must be a number")
		}
		topK = parsed
	} else if value, exists := payload["count"]; exists {
		parsed, err := intFromJSONNumber(value)
		if err != nil {
			return searchRequestParams{}, fmt.Errorf("count must be a number")
		}
		topK = parsed
	}
	if topK < 1 {
		topK = 1
	}

	expand := false
	if rawExpand, ok := payload["expand"]; ok {
		value, ok := rawExpand.(bool)
		if !ok {
			return searchRequestParams{}, fmt.Errorf("expand must be a boolean")
		}
		expand = value
	}

	hybrid := true
	if rawHybrid, ok := payload["hybrid"]; ok {
		value, ok := rawHybrid.(bool)
		if !ok {
			return searchRequestParams{}, fmt.Errorf("hybrid must be a boolean")
		}
		hybrid = value
	}

	category := ""
	if rawCategory, ok := payload["category"]; ok {
		value, ok := rawCategory.(string)
		if !ok {
			return searchRequestParams{}, fmt.Errorf("category must be a string")
		}
		category = strings.TrimSpace(value)
	}

	alpha := defaultHybridAlpha
	if rawAlpha, ok := payload["alpha"]; ok {
		value, err := floatFromJSONNumber(rawAlpha)
		if err != nil {
			return searchRequestParams{}, fmt.Errorf("alpha must be a number")
		}
		alpha = value
	}

	beta := defaultHybridBeta
	if rawBeta, ok := payload["beta"]; ok {
		value, err := floatFromJSONNumber(rawBeta)
		if err != nil {
			return searchRequestParams{}, fmt.Errorf("beta must be a number")
		}
		beta = value
	}

	decayRadiusKm := defaultHybridDecayRadiusKm
	if rawDecayRadius, ok := payload["decay_radius_km"]; ok {
		value, err := floatFromJSONNumber(rawDecayRadius)
		if err != nil {
			return searchRequestParams{}, fmt.Errorf("decay_radius_km must be a number")
		}
		decayRadiusKm = value
	}

	centerLat, centerLng, err := parseCenterCoordinates(payload)
	if err != nil {
		return searchRequestParams{}, err
	}

	return searchRequestParams{
		Query:         strings.TrimSpace(query),
		TopK:          topK,
		Expand:        expand,
		Hybrid:        hybrid,
		Category:      category,
		Alpha:         alpha,
		Beta:          beta,
		DecayRadiusKm: decayRadiusKm,
		CenterLat:     centerLat,
		CenterLng:     centerLng,
	}, nil
}

func parseCenterCoordinates(payload map[string]any) (*float64, *float64, error) {
	rawLat, latExists := payload["center_lat"]
	rawLng, lngExists := payload["center_lng"]
	if !latExists && !lngExists {
		return nil, nil, nil
	}
	if !latExists || !lngExists {
		return nil, nil, fmt.Errorf("center_lat and center_lng must be provided together")
	}

	lat, err := floatFromJSONNumber(rawLat)
	if err != nil {
		return nil, nil, fmt.Errorf("center_lat must be a number")
	}
	lng, err := floatFromJSONNumber(rawLng)
	if err != nil {
		return nil, nil, fmt.Errorf("center_lng must be a number")
	}
	return &lat, &lng, nil
}

func decodeSearchResults(respBody []byte) ([]SearchResult, string, bool) {
	if len(respBody) == 0 || !json.Valid(respBody) {
		return nil, "", false
	}

	var envelope upstreamSearchEnvelope
	if err := json.Unmarshal(respBody, &envelope); err == nil {
		if results, ok := decodeSearchRows(envelope.Results); ok {
			return results, strings.TrimSpace(envelope.ExpandedQuery), true
		}
		if results, ok := decodeSearchRows(envelope.Rows); ok {
			return results, strings.TrimSpace(envelope.ExpandedQuery), true
		}
	}

	if results, ok := decodeSearchRows(respBody); ok {
		return results, "", true
	}

	return nil, "", false
}

func decodeSearchRows(raw json.RawMessage) ([]SearchResult, bool) {
	if len(raw) == 0 {
		return nil, false
	}

	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, false
	}

	results := make([]SearchResult, 0, len(rows))
	for i, row := range rows {
		results = append(results, normalizeSearchResult(row, i))
	}
	return results, true
}

func normalizeSearchResult(row map[string]any, index int) SearchResult {
	id := stringFromAny(row["id"])
	if id == "" {
		id = fmt.Sprintf("row-%d", index)
	}
	embedText := stringFromAny(row["embed_text"])
	if embedText == "" {
		embedText = stringFromAny(row["name"])
	}
	category := stringFromAny(row["category"])
	country := stringFromAny(row["country"])
	geom := normalizeGeometryValue(row["geom"])
	distance := float64FromAny(row["distance"])
	if !isFinite(distance) {
		distance = float64FromAny(row["score"])
	}
	raw, _ := json.Marshal(row)
	return SearchResult{
		ID:        id,
		EmbedText: embedText,
		Geom:      geom,
		Category:  category,
		Country:   country,
		Distance:  distance,
		Raw:       raw,
		Extra:     row,
	}
}

func normalizeGeometryValue(value any) any {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case json.RawMessage:
		return json.RawMessage(v)
	case map[string]any:
		return v
	default:
		if value == nil {
			return ""
		}
		return value
	}
}

func marshalRankedResults(results []ScoredResult, wrapExpandedQuery bool, expandedQuery string) ([]byte, error) {
	if wrapExpandedQuery {
		return json.Marshal(struct {
			ExpandedQuery string         `json:"expanded_query"`
			Results       []ScoredResult `json:"results"`
		}{ExpandedQuery: expandedQuery, Results: results})
	}
	return json.Marshal(results)
}

func defaultSearchTopK(payload map[string]any) int {
	if value, exists := payload["top_k"]; exists {
		if parsed, err := intFromJSONNumber(value); err == nil && parsed > 0 {
			return parsed
		}
	}
	if value, exists := payload["count"]; exists {
		if parsed, err := intFromJSONNumber(value); err == nil && parsed > 0 {
			return parsed
		}
	}
	return 5
}

func intFromJSONNumber(value any) (int, error) {
	parsed, err := floatFromJSONNumber(value)
	if err != nil {
		return 0, err
	}
	if math.Trunc(parsed) != parsed {
		return 0, fmt.Errorf("expected integer")
	}
	return int(parsed), nil
}

func floatFromJSONNumber(value any) (float64, error) {
	switch v := value.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case uint:
		return float64(v), nil
	case uint32:
		return float64(v), nil
	case uint64:
		return float64(v), nil
	case json.Number:
		return v.Float64()
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return 0, fmt.Errorf("empty number")
		}
		return strconv.ParseFloat(trimmed, 64)
	default:
		return 0, fmt.Errorf("unsupported number type %T", value)
	}
}

func stringFromAny(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	case json.Number:
		return v.String()
	default:
		return ""
	}
}

func float64FromAny(value any) float64 {
	parsed, err := floatFromJSONNumber(value)
	if err != nil {
		return 0
	}
	return parsed
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
