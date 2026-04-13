package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"examle.com/mod/config"
)

const (
	geminiAPIBaseURL        = "https://generativelanguage.googleapis.com/v1beta/models"
	queryExpansionModel     = "gemini-2.5-flash"
	queryExpansionTimeout   = 5 * time.Second
	queryExpansionMaxTokens = 150
)

const queryExpansionSystemPrompt = `You are a query expansion assistant for a semantic search engine.
      Your job is to rewrite a short user query into a richer, more
      descriptive version that will improve semantic search recall.
      Return only the expanded query, nothing else. No explanations,
      no preamble, no quotation marks.`

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiGenerationConfig struct {
	MaxOutputTokens int `json:"maxOutputTokens"`
}

type geminiGenerateContentRequest struct {
	SystemInstruction geminiContent          `json:"system_instruction"`
	Contents          []geminiContent        `json:"contents"`
	GenerationConfig  geminiGenerationConfig `json:"generationConfig"`
}

type geminiGenerateContentResponse struct {
	Candidates []struct {
		Content geminiContent `json:"content"`
	} `json:"candidates"`
}

var geminiHTTPClient = &http.Client{Timeout: queryExpansionTimeout}

func QueryExpansionEnabled() bool {
	return strings.TrimSpace(config.Current().GeminiAPIKey) != ""
}

func ExpandQuery(query string) (string, error) {
	original := strings.TrimSpace(query)
	apiKey := strings.TrimSpace(config.Current().GeminiAPIKey)
	if original == "" {
		return query, fmt.Errorf("query must be a non-empty string")
	}
	if apiKey == "" {
		return query, fmt.Errorf("gemini api key is not configured")
	}

	payload := geminiGenerateContentRequest{
		SystemInstruction: geminiContent{Parts: []geminiPart{{Text: queryExpansionSystemPrompt}}},
		Contents: []geminiContent{{
			Role:  "user",
			Parts: []geminiPart{{Text: fmt.Sprintf("Expand this search query for semantic search: %s", original)}},
		}},
		GenerationConfig: geminiGenerationConfig{MaxOutputTokens: queryExpansionMaxTokens},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return query, fmt.Errorf("failed to marshal gemini request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/%s:generateContent?key=%s", geminiAPIBaseURL, queryExpansionModel, url.QueryEscape(apiKey))
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return query, fmt.Errorf("failed to build gemini request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := geminiHTTPClient.Do(req)
	if err != nil {
		return query, fmt.Errorf("gemini request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return query, fmt.Errorf("failed to read gemini response: %w", err)
	}
	if resp.StatusCode >= http.StatusMultipleChoices {
		return query, fmt.Errorf("gemini request returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var parsed geminiGenerateContentResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return query, fmt.Errorf("failed to parse gemini response: %w", err)
	}

	var expandedParts []string
	for _, candidate := range parsed.Candidates {
		for _, part := range candidate.Content.Parts {
			if strings.TrimSpace(part.Text) != "" {
				expandedParts = append(expandedParts, part.Text)
			}
		}
	}
	expanded := strings.TrimSpace(strings.Join(expandedParts, ""))
	if expanded == "" {
		return query, fmt.Errorf("gemini returned an empty expansion")
	}

	return expanded, nil
}
