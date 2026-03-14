package handlers

import (
	"fmt"
	"math"
	"net/http"
	"strings"

	"examle.com/mod/services"
	"github.com/gin-gonic/gin"
)

// IndexFile handles POST /api/v1/index-file.
//
// It validates the incoming bbox request and forwards it to the main backend's
// /api/v1/semantic-geo-search/index endpoint for asynchronous indexing.
func IndexFile(c *gin.Context) {
	var req services.IndexRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	req.S3Path = strings.TrimSpace(req.S3Path)
	if req.S3Path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "s3_path is required"})
		return
	}

	req.Region = strings.TrimSpace(req.Region)
	if req.Region == "" {
		req.Region = defaultRegion
	}

	if err := validateBBox(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := services.SendIndexRequest(c.Request.Context(), req); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to start indexing: " + err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"message":    "indexing started",
		"s3_path":    req.S3Path,
		"region":     req.Region,
		"bbox_min_x": req.BBoxMinX,
		"bbox_max_x": req.BBoxMaxX,
		"bbox_min_y": req.BBoxMinY,
		"bbox_max_y": req.BBoxMaxY,
		"all":        req.All,
		"count":      req.Count,
	})
}

func validateBBox(req services.IndexRequest) error {
	if math.IsNaN(req.BBoxMinX) || math.IsNaN(req.BBoxMaxX) || math.IsNaN(req.BBoxMinY) || math.IsNaN(req.BBoxMaxY) {
		return fmt.Errorf("bbox values must be valid numbers")
	}
	if math.IsInf(req.BBoxMinX, 0) || math.IsInf(req.BBoxMaxX, 0) || math.IsInf(req.BBoxMinY, 0) || math.IsInf(req.BBoxMaxY, 0) {
		return fmt.Errorf("bbox values must be finite numbers")
	}
	if req.BBoxMinX >= req.BBoxMaxX {
		return fmt.Errorf("bbox_min_x must be less than bbox_max_x")
	}
	if req.BBoxMinY >= req.BBoxMaxY {
		return fmt.Errorf("bbox_min_y must be less than bbox_max_y")
	}
	return nil
}
