package handlers

import (
	"encoding/json"
	"io"
	"net/http"

	"examle.com/mod/services"
	"github.com/gin-gonic/gin"
)

// Search handles POST /search.
// It validates the incoming payload and forwards it to the main backend's
// /semantic-geo-search/ endpoint.
func Search(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}
	if len(body) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "request body is required"})
		return
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON payload"})
		return
	}
	if _, err := services.NormalizeSearchPayload(payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	status, respBody, err := services.ForwardSearchRequest(c.Request.Context(), payload)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "search request failed: " + err.Error()})
		return
	}

	if json.Valid(respBody) {
		c.Data(status, "application/json", respBody)
		return
	}
	c.Data(status, "text/plain; charset=utf-8", respBody)
}
