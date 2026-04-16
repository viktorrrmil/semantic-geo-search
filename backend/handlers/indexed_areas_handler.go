package handlers

import (
	"encoding/json"
	"net/http"

	"examle.com/mod/services"
	"github.com/gin-gonic/gin"
)

// GetIndexedAreas handles GET /api/v1/geo/indexed-areas.
// It forwards the request to the main backend and streams back the response.
func GetIndexedAreas(c *gin.Context) {
	status, respBody, err := services.ForwardIndexedAreasRequest(c.Request.Context(), c.Request.URL.RawQuery)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "indexed areas request failed: " + err.Error()})
		return
	}

	if json.Valid(respBody) {
		c.Data(status, "application/json", respBody)
		return
	}
	c.Data(status, "text/plain; charset=utf-8", respBody)
}
