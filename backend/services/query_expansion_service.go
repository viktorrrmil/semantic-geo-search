package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
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
	queryExpansionMaxTokens = 256
)

const queryExpansionSystemPrompt = `You are a query expansion assistant for a semantic search engine.
	  Your job is to enrich a short user query without losing any of its
	  original meaning or context. Preserve all named entities, locations,
	  regions, countries, landmarks, numbers, dates, and other qualifiers
	  verbatim. Add only additional relevant descriptors that make the query
	  richer for semantic search. Return a single search query only, with no
	  explanations, labels, bullets, or prompt text.`

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
	SystemInstruction geminiContent          `json:"systemInstruction"`
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
	log.Printf("[query-expansion] start query=%q len=%d", previewQuery(original), len(original))
	if original == "" {
		log.Printf("[query-expansion] stop: empty query")
		return query, fmt.Errorf("query must be a non-empty string")
	}
	if apiKey == "" {
		log.Printf("[query-expansion] stop: gemini api key is not configured")
		return query, fmt.Errorf("gemini api key is not configured")
	}

	payload := geminiGenerateContentRequest{
		SystemInstruction: geminiContent{Parts: []geminiPart{{Text: queryExpansionSystemPrompt}}},
		Contents: []geminiContent{{
			Role:  "user",
			Parts: []geminiPart{{Text: original}},
		}},
		GenerationConfig: geminiGenerationConfig{MaxOutputTokens: queryExpansionMaxTokens},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[query-expansion] stop: marshal request failed: %v", err)
		return query, fmt.Errorf("failed to marshal gemini request: %w", err)
	}
	log.Printf("[query-expansion] request marshaled body_bytes=%d", len(body))

	endpoint := fmt.Sprintf("%s/%s:generateContent?key=%s", geminiAPIBaseURL, queryExpansionModel, url.QueryEscape(apiKey))
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		log.Printf("[query-expansion] stop: build request failed: %v", err)
		return query, fmt.Errorf("failed to build gemini request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	log.Printf("[query-expansion] sending request model=%s", queryExpansionModel)

	resp, err := geminiHTTPClient.Do(req)
	if err != nil {
		log.Printf("[query-expansion] stop: request failed: %v", err)
		return query, fmt.Errorf("gemini request failed: %w", err)
	}
	defer resp.Body.Close()
	log.Printf("[query-expansion] response received status=%d", resp.StatusCode)

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[query-expansion] stop: read response failed: %v", err)
		return query, fmt.Errorf("failed to read gemini response: %w", err)
	}
	if resp.StatusCode >= http.StatusMultipleChoices {
		log.Printf("[query-expansion] stop: non-success status=%d body=%q", resp.StatusCode, previewQuery(strings.TrimSpace(string(respBody))))
		return query, fmt.Errorf("gemini request returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var parsed geminiGenerateContentResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		log.Printf("[query-expansion] stop: parse response failed: %v", err)
		return query, fmt.Errorf("failed to parse gemini response: %w", err)
	}
	log.Printf("[query-expansion] parsed candidates=%d", len(parsed.Candidates))

	var expandedParts []string
	for _, candidate := range parsed.Candidates {
		for _, part := range candidate.Content.Parts {
			if strings.TrimSpace(part.Text) != "" {
				expandedParts = append(expandedParts, part.Text)
			}
		}
	}
	expanded := strings.TrimSpace(strings.Join(expandedParts, ""))
	log.Printf("[query-expansion] raw expansion=%q len=%d", previewQuery(expanded), len(expanded))
	expanded = sanitizeExpandedQuery(original, expanded)
	log.Printf("[query-expansion] sanitized expansion=%q len=%d", previewQuery(expanded), len(expanded))
	if expanded == "" {
		log.Printf("[query-expansion] stop: empty expansion after sanitization")
		return query, fmt.Errorf("gemini returned an empty expansion")
	}

	finalQuery := preserveOriginalQuery(original, expanded)
	log.Printf("[query-expansion] done final=%q len=%d", previewQuery(finalQuery), len(finalQuery))
	return finalQuery, nil
}

func previewQuery(value string) string {
	const maxPreviewLen = 180
	value = strings.TrimSpace(value)
	if len(value) <= maxPreviewLen {
		return value
	}
	return value[:maxPreviewLen] + "..."
}

func sanitizeExpandedQuery(original, expanded string) string {
	expanded = strings.TrimSpace(expanded)
	if expanded == "" {
		return ""
	}

	lower := strings.ToLower(expanded)
	for _, marker := range []string{
		"expand this search query",
		"original query:",
		"keep the original query intact",
		"do not remove, replace, or summarize",
		"return a single search query only",
		"query expansion assistant",
	} {
		if strings.Contains(lower, marker) {
			return strings.TrimSpace(original)
		}
	}

	return expanded
}

func normalizeQueryText(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func preserveOriginalQuery(original, expanded string) string {
	original = strings.TrimSpace(original)
	expanded = strings.TrimSpace(expanded)
	if original == "" {
		return expanded
	}
	if expanded == "" {
		return original
	}

	normalizedOriginal := normalizeQueryText(original)
	normalizedExpanded := normalizeQueryText(expanded)
	if normalizedOriginal != "" && strings.Contains(normalizedExpanded, normalizedOriginal) {
		return expanded
	}

	return strings.TrimSpace(original + " " + expanded)
}
