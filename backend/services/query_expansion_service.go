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
	"time"
)

const (
	anthropicMessagesURL    = "https://api.anthropic.com/v1/messages"
	queryExpansionModel     = "claude-sonnet-4-20250514"
	queryExpansionTimeout   = 5 * time.Second
	anthropicAPIVersion     = "2023-06-01"
	queryExpansionMaxTokens = 150
)

const queryExpansionSystemPrompt = `You are a query expansion assistant for a semantic search engine.
      Your job is to rewrite a short user query into a richer, more
      descriptive version that will improve semantic search recall.
      Return only the expanded query, nothing else. No explanations,
      no preamble, no quotation marks.`

type anthropicMessage struct {
	Role    string             `json:"role"`
	Content []anthropicContent `json:"content"`
}

type anthropicContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicMessagesRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessagesResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

var anthropicHTTPClient = &http.Client{Timeout: queryExpansionTimeout}

func QueryExpansionEnabled() bool {
	return strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")) != ""
}

func ExpandQuery(query string) (string, error) {
	original := strings.TrimSpace(query)
	apiKey := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
	if original == "" {
		return query, fmt.Errorf("query must be a non-empty string")
	}
	if apiKey == "" {
		return query, fmt.Errorf("anthropic api key is not configured")
	}

	payload := anthropicMessagesRequest{
		Model:     queryExpansionModel,
		MaxTokens: queryExpansionMaxTokens,
		System:    queryExpansionSystemPrompt,
		Messages: []anthropicMessage{{
			Role: "user",
			Content: []anthropicContent{{
				Type: "text",
				Text: fmt.Sprintf("Expand this search query for semantic search: %s", original),
			}},
		}},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return query, fmt.Errorf("failed to marshal anthropic request: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, anthropicMessagesURL, bytes.NewReader(body))
	if err != nil {
		return query, fmt.Errorf("failed to build anthropic request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", anthropicAPIVersion)

	resp, err := anthropicHTTPClient.Do(req)
	if err != nil {
		return query, fmt.Errorf("anthropic request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return query, fmt.Errorf("failed to read anthropic response: %w", err)
	}
	if resp.StatusCode >= http.StatusMultipleChoices {
		return query, fmt.Errorf("anthropic request returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var parsed anthropicMessagesResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return query, fmt.Errorf("failed to parse anthropic response: %w", err)
	}

	var expandedParts []string
	for _, part := range parsed.Content {
		if strings.EqualFold(part.Type, "text") && strings.TrimSpace(part.Text) != "" {
			expandedParts = append(expandedParts, part.Text)
		}
	}
	expanded := strings.TrimSpace(strings.Join(expandedParts, ""))
	if expanded == "" {
		return query, fmt.Errorf("anthropic returned an empty expansion")
	}

	return expanded, nil
}
