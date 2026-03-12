package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"examle.com/mod/services"
	"github.com/gin-gonic/gin"
)

const (
	defaultK      = 100
	maxK          = 10_000
	defaultRegion = "us-west-2"
)

// GetSpatialData handles GET /api/v1/spatial-data
//
// Query parameters:
//
//	path   (required) – S3 or local path to a Parquet file.
//	                    e.g. s3://overturemaps-us-west-2/release/2026-01-21.0/theme=buildings/type=building/part-00000.parquet
//	k      (optional) – number of rows to return (default 100, max 10 000).
//	region (optional) – AWS region for S3 access (default us-west-2).
//
// Geometry columns are returned as WKT strings.
func GetSpatialData(c *gin.Context) {
	path := strings.TrimSpace(c.Query("path"))
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query param 'path' is required"})
		return
	}

	k := defaultK
	if kStr := c.Query("k"); kStr != "" {
		parsed, err := strconv.Atoi(kStr)
		if err != nil || parsed <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "'k' must be a positive integer"})
			return
		}
		if parsed > maxK {
			c.JSON(http.StatusBadRequest, gin.H{"error": "'k' exceeds the maximum allowed value of 10 000"})
			return
		}
		k = parsed
	}

	region := c.DefaultQuery("region", defaultRegion)

	svc, err := services.GetDuckDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "DuckDB service unavailable: " + err.Error()})
		return
	}

	rows, err := svc.QueryFile(c.Request.Context(), path, region, k)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"path":  path,
		"count": len(rows),
		"rows":  rows,
	})
}
