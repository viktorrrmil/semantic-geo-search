package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// IndexDoc is a single document sent to the vector-search backend.
type IndexDoc struct {
	ID       string         `json:"id"`
	Text     string         `json:"text"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// EmbeddingBatchRequest is the payload sent to the vector-search backend's /add-batch endpoint.
type EmbeddingBatchRequest struct {
	Batch []string `json:"batch"`
}

// BatchResponse is what the vector-search backend returns.
type BatchResponse struct {
	Indexed int    `json:"indexed"`
	Message string `json:"message,omitempty"`
}

var indexerHTTPClient = &http.Client{Timeout: 60 * time.Second}

// SendBatch POSTs docs to the vector-search backend's /add-batch endpoint.
func SendBatch(ctx context.Context, batchURL string, docs []string) (*BatchResponse, error) {
	payload, err := json.Marshal(EmbeddingBatchRequest{Batch: docs})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal batch: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, batchURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := indexerHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("batch request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("batch endpoint returned %d: %s", resp.StatusCode, body)
	}

	var result BatchResponse
	if err := json.Unmarshal(body, &result); err != nil {
		// Non-fatal: endpoint may return a different shape; still report success.
		result.Message = string(body)
	}
	return &result, nil
}

// BuildRowText converts a row map into a dense, human-readable text string
// suitable for embedding.  Null/empty values are omitted.  Nested structures
// are flattened with dot-notation keys.  The output is deterministic (keys
// sorted alphabetically) so embeddings are reproducible.
func BuildRowText(row map[string]any) string {
	lines := flattenMap("", row)
	sort.Strings(lines)

	var b strings.Builder
	for _, line := range lines {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}

// flattenMap recursively turns a nested map into "prefix.key: value" lines.
func flattenMap(prefix string, m map[string]any) []string {
	var lines []string
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		lines = append(lines, flattenValue(key, v)...)
	}
	return lines
}

func flattenValue(key string, v any) []string {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case map[string]any:
		return flattenMap(key, val)
	case []any:
		var lines []string
		for i, item := range val {
			lines = append(lines, flattenValue(fmt.Sprintf("%s[%d]", key, i), item)...)
		}
		return lines
	case string:
		if strings.TrimSpace(val) == "" {
			return nil
		}
		return []string{fmt.Sprintf("%s: %s", key, val)}
	case bool:
		return []string{fmt.Sprintf("%s: %t", key, val)}
	case float64:
		return []string{fmt.Sprintf("%s: %g", key, val)}
	case int64:
		return []string{fmt.Sprintf("%s: %d", key, val)}
	case int32:
		return []string{fmt.Sprintf("%s: %d", key, val)}
	default:
		s := fmt.Sprintf("%v", val)
		if s == "" || s == "<nil>" || s == "map[]" {
			return nil
		}
		return []string{fmt.Sprintf("%s: %s", key, s)}
	}
}
