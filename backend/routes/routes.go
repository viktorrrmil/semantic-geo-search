package routes

import (
	"examle.com/mod/handlers"
	"github.com/gin-gonic/gin"
)

// Register mounts all API routes onto the provided engine.
func Register(r *gin.Engine) {
	r.POST("/search", handlers.Search)

	v1 := r.Group("/api/v1")
	{
		// GET /api/v1/list-files/*url
		// Lists immediate children of an S3 bucket/prefix.
		// Examples:
		//   /api/v1/list-files/overturemaps-us-west-2/release
		//   /api/v1/list-files/s3://overturemaps-us-west-2/release
		v1.GET("/list-files/*url", handlers.ListS3Files)

		// GET /api/v1/spatial-data?path=<s3-or-local>&k=<rows>&region=<aws-region>
		// Reads k rows from a Parquet file via DuckDB.
		// Geometry columns are returned as WKT strings.
		v1.GET("/spatial-data", handlers.GetSpatialData)

		// POST /api/v1/index-file
		// Accepts an S3 path, region, bbox bounds, and "all" flag, then forwards
		// the request to the main backend for asynchronous indexing.
		v1.POST("/index-file", handlers.IndexFile)
	}
}
