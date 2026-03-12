package handlers

import (
	"net/http"
	"strings"

	"examle.com/mod/services"
	"github.com/gin-gonic/gin"
)

// ListS3Files handles GET /api/v1/list-files/*url
//
// The wildcard captures everything after /list-files/, which should be an
// S3 path in one of these forms:
//
//	s3://bucket/prefix/...
//	bucket/prefix/...
//
// An optional query param ?region= overrides the AWS region (default us-west-2).
func ListS3Files(c *gin.Context) {
	rawPath := c.Param("url")
	rawPath = strings.TrimPrefix(rawPath, "/")

	rawPath = strings.TrimPrefix(rawPath, "s3://")

	if rawPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "S3 path is required"})
		return
	}

	// Split into bucket and prefix.
	parts := strings.SplitN(rawPath, "/", 2)
	bucket := parts[0]
	prefix := ""
	if len(parts) == 2 {
		prefix = parts[1]
	}

	region := c.DefaultQuery("region", "us-west-2")

	entries, err := services.ListS3Entries(c.Request.Context(), bucket, prefix, region)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"bucket":  bucket,
		"prefix":  prefix,
		"entries": entries,
	})
}
