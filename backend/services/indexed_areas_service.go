package services

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

const indexedAreasEndpointPath = "/api/v1/geo/indexed-areas"

func ForwardIndexedAreasRequest(ctx context.Context, rawQuery string) (int, []byte, error) {
	targetURL := mainBackendURL() + indexedAreasEndpointPath
	if rawQuery != "" {
		targetURL += "?" + rawQuery
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to build indexed areas request: %w", err)
	}

	resp, err := indexerHTTPClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("indexed areas request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body, nil
}
