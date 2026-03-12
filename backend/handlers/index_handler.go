package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"examle.com/mod/services"
	"github.com/gin-gonic/gin"
)

const (
	defaultIndexK      = 100
	defaultIndexRegion = "us-west-2"
	addBatchURL        = "http://localhost:8080/vector_store/add_batch"
)

// IndexFileRequest is the payload for POST /api/v1/index-file.
type IndexFileRequest struct {
	S3Path string `json:"s3_path" binding:"required"`
	Count  int    `json:"count"`
	Region string `json:"region"`
}

// IndexFile handles POST /api/v1/index-file.
//
// It fetches up to Count rows from the Parquet file at S3Path using DuckDB,
// converts each row into a rich text string, then POSTs the whole batch to the
// vector-search backend at localhost:8080/add-batch for embedding and indexing.
func IndexFile(c *gin.Context) {
	var req IndexFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	req.S3Path = strings.TrimSpace(req.S3Path)
	if req.Region == "" {
		req.Region = defaultIndexRegion
	}
	if req.Count <= 0 {
		req.Count = defaultIndexK
	}
	if req.Count > maxK {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("count exceeds maximum of %d", maxK)})
		return
	}

	// Fetch rows from the Parquet file via DuckDB.
	svc, err := services.GetDuckDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "DuckDB service unavailable: " + err.Error()})
		return
	}

	rows, err := svc.QueryFile(c.Request.Context(), req.S3Path, req.Region, req.Count)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query file: " + err.Error()})
		return
	}

	if len(rows) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"message": "no rows found",
			"s3_path": req.S3Path,
			"indexed": 0,
		})
		return
	}

	// Convert each row to an IndexDoc (id + text representation).
	docs := make([]string, 0, len(rows))
	for _, row := range rows {
		//id := extractID(row)
		text := services.BuildRowText(row)
		if text == "" {
			continue
		}
		//docs = append(docs, services.IndexDoc{
		//	ID:   id,
		//	Text: text,
		//	Metadata: map[string]any{
		//		"source": req.S3Path,
		//	},
		//})
		docs = append(docs, text)
	}

	// Send the batch to the vector-search backend.
	batchResp, err := services.SendBatch(c.Request.Context(), addBatchURL, docs)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "failed to send batch to search backend: " + err.Error(),
			"indexed": 0,
			"built":   len(docs),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"s3_path":        req.S3Path,
		"fetched":        len(rows),
		"sent":           len(docs),
		"batch_response": batchResp,
	})
}

// extractID pulls the row's id field, falling back to a sequential placeholder.
func extractID(row map[string]any) string {
	for _, key := range []string{"id", "ID", "uuid", "fid", "osm_id"} {
		if v, ok := row[key]; ok && v != nil {
			return fmt.Sprintf("%v", v)
		}
	}
	return fmt.Sprintf("row-%p", &row)
}
