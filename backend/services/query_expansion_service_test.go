package services

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPreserveOriginalQuery(t *testing.T) {
	tests := []struct {
		name     string
		original string
		expanded string
		want     string
	}{
		{
			name:     "prepends missing location context",
			original: "cafe in barcelona",
			expanded: "cozy cafe with comfortable atmosphere",
			want:     "cafe in barcelona cozy cafe with comfortable atmosphere",
		},
		{
			name:     "keeps expanded query when original already preserved",
			original: "best sushi in tokyo",
			expanded: "best sushi in tokyo with modern ambiance and fresh fish",
			want:     "best sushi in tokyo with modern ambiance and fresh fish",
		},
		{
			name:     "trims surrounding whitespace",
			original: "  parks in new york  ",
			expanded: "  family friendly outdoor spaces  ",
			want:     "parks in new york family friendly outdoor spaces",
		},
		{
			name:     "falls back to original when expansion is empty",
			original: "museum in london",
			expanded: "   ",
			want:     "museum in london",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := preserveOriginalQuery(tc.original, tc.expanded)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSanitizeExpandedQuery(t *testing.T) {
	tests := []struct {
		name     string
		original string
		expanded string
		want     string
	}{
		{
			name:     "rejects prompt echo",
			original: "cafe in barcelona",
			expanded: "Expand this search query for semantic search. Original query: cafe in barcelona",
			want:     "cafe in barcelona",
		},
		{
			name:     "keeps normal expansion",
			original: "cafe in barcelona",
			expanded: "cafe in barcelona cozy cafe with comfortable atmosphere",
			want:     "cafe in barcelona cozy cafe with comfortable atmosphere",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeExpandedQuery(tc.original, tc.expanded)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGeminiGenerateContentRequestUsesExpectedFieldNames(t *testing.T) {
	payload := geminiGenerateContentRequest{
		SystemInstruction: geminiContent{Parts: []geminiPart{{Text: "system prompt"}}},
		Contents: []geminiContent{{Role: "user", Parts: []geminiPart{{Text: "cafe in barcelona"}}}},
		GenerationConfig: geminiGenerationConfig{MaxOutputTokens: queryExpansionMaxTokens},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	got := string(data)
	for _, want := range []string{`"systemInstruction"`, `"generationConfig"`, `"maxOutputTokens":256`} {
		if !strings.Contains(got, want) {
			t.Fatalf("payload missing %q: %s", want, got)
		}
	}
}

