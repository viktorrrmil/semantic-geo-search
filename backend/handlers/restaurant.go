package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"examle.com/mod/models"
	"examle.com/mod/services"
	"github.com/gin-gonic/gin"
)

// RestaurantHandler handles restaurant-related API endpoints
type RestaurantHandler struct {
	duckdbService *services.DuckDBService
}

// NewRestaurantHandler creates a new restaurant handler
func NewRestaurantHandler(duckdbService *services.DuckDBService) *RestaurantHandler {
	return &RestaurantHandler{
		duckdbService: duckdbService,
	}
}

// Search handles POST /search endpoint
func (h *RestaurantHandler) Search(c *gin.Context) {
	var req models.SearchRequest

	// Bind JSON request body
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.SearchResponse{
			Message: "Invalid request body: " + err.Error(),
		})
		return
	}

	// Set default limit if not provided
	if req.Limit <= 0 {
		req.Limit = 10
	}

	// For now, return sample data to avoid long query times
	// In production, you would use: restaurants, err := h.duckdbService.SearchRestaurants(req)
	restaurants := h.duckdbService.GetSamplePizzaRestaurants()

	// Limit the results to the requested amount
	if len(restaurants) > req.Limit {
		restaurants = restaurants[:req.Limit]
	}

	response := models.SearchResponse{
		Restaurants: restaurants,
		Total:       len(restaurants),
		Message:     "Sample data returned for quick response",
	}

	c.JSON(http.StatusOK, response)
}

// SearchReal handles POST /search-real endpoint for actual Overture data
func (h *RestaurantHandler) SearchReal(c *gin.Context) {
	var req models.SearchRequest

	// Bind JSON request body
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.SearchResponse{
			Message: "Invalid request body: " + err.Error(),
		})
		return
	}

	// Set default limit if not provided
	if req.Limit <= 0 {
		req.Limit = 10
	}

	restaurants, err := h.duckdbService.SearchRestaurants(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.SearchResponse{
			Message: "Failed to search restaurants: " + err.Error(),
		})
		return
	}

	response := models.SearchResponse{
		Restaurants: restaurants,
		Total:       len(restaurants),
		Message:     fmt.Sprintf("Found %d restaurants using real Overture data", len(restaurants)),
	}

	c.JSON(http.StatusOK, response)
}

// Health handles GET /health endpoint
func (h *RestaurantHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "healthy",
		"message": "Semantic Geo Search API is running",
	})
}

// PreprocessChunk handles POST /preprocess endpoint
func (h *RestaurantHandler) PreprocessChunk(c *gin.Context) {
	log.Printf("🚀 Received preprocessing request from %s", c.ClientIP())

	var req models.PreprocessRequest

	// Bind JSON request body
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("❌ Invalid request body: %v", err)
		c.JSON(http.StatusBadRequest, models.PreprocessResponse{
			Message: "Invalid request body: " + err.Error(),
		})
		return
	}

	log.Printf("📋 Request details:")
	log.Printf("   Category: %s", req.Category)
	log.Printf("   Bounding box: [%.6f, %.6f, %.6f, %.6f]", req.BboxMinX, req.BboxMinY, req.BboxMaxX, req.BboxMaxY)
	if req.ChunkID != "" {
		log.Printf("   Custom chunk ID: %s", req.ChunkID)
	}
	log.Println("🔄 Starting preprocessing operation...")

	// Preprocess the chunk
	startTime := time.Now()
	response, err := h.duckdbService.PreprocessChunk(req)
	processingDuration := time.Since(startTime)

	if err != nil {
		log.Printf("❌ Preprocessing failed after %.2f seconds: %v", processingDuration.Seconds(), err)
		c.JSON(http.StatusInternalServerError, models.PreprocessResponse{
			Message: "Failed to preprocess chunk: " + err.Error(),
		})
		return
	}

	log.Printf("✅ Preprocessing completed successfully!")
	log.Printf("   Duration: %.2f seconds", processingDuration.Seconds())
	log.Printf("   Records processed: %d", response.RecordCount)
	log.Printf("   Chunk ID: %s", response.ChunkID)
	log.Printf("   Response sent to client: %s", c.ClientIP())

	c.JSON(http.StatusOK, response)
}

// GetAvailableChunks handles GET /chunks endpoint
func (h *RestaurantHandler) GetAvailableChunks(c *gin.Context) {
	response, err := h.duckdbService.GetAvailableChunks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ChunksListResponse{
			Message: "Failed to get available chunks: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

// SearchChunk handles POST /search-chunk/:chunkId endpoint
func (h *RestaurantHandler) SearchChunk(c *gin.Context) {
	chunkID := c.Param("chunkId")
	fmt.Printf("🔍 Received search request for chunk ID: %s from %s\n", chunkID, c.ClientIP())
	if chunkID == "" {
		c.JSON(http.StatusBadRequest, models.SearchResponse{
			Message: "Chunk ID is required",
		})
		return
	}

	// Get limit from query parameter
	limit := 10
	if limitStr := c.Query("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	// Attempt fast parquet-backed search using stored bbox metadata
	meta, metaErr := h.duckdbService.GetChunkMetadata(chunkID)
	if metaErr == nil {
		// Try parquet search with stored bbox
		restaurants, err := h.duckdbService.SearchParquetChunk(chunkID, meta.BboxMinX, meta.BboxMinY, meta.BboxMaxX, meta.BboxMaxY, limit)
		if err == nil {
			response := models.SearchResponse{
				Restaurants: restaurants,
				Total:       len(restaurants),
				Message:     fmt.Sprintf("Found %d restaurants from parquet chunk %s", len(restaurants), chunkID),
			}

			c.JSON(http.StatusOK, response)
			return
		}
		// Log parquet search failure and fall back
		log.Printf("parquet search failed for chunk %s: %v; falling back to JSON", chunkID, err)
	} else {
		log.Printf("failed to load chunk metadata for %s: %v; falling back to JSON", chunkID, metaErr)
	}

	// Fallback: search in-memory using JSON chunk
	restaurants, err := h.duckdbService.SearchFromChunk(chunkID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.SearchResponse{
			Message: "Failed to search chunk: " + err.Error(),
		})
		return
	}

	response := models.SearchResponse{
		Restaurants: restaurants,
		Total:       len(restaurants),
		Message:     fmt.Sprintf("Found %d restaurants from precomputed chunk %s (fallback)", len(restaurants), chunkID),
	}

	c.JSON(http.StatusOK, response)
}

// GetChunkGeoJSON handles GET /chunks/:chunkId/geojson and returns the chunk as GeoJSON
func (h *RestaurantHandler) GetChunkGeoJSON(c *gin.Context) {
	chunkID := c.Param("chunkId")
	if chunkID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Chunk ID is required"})
		return
	}

	// Try fast export (parquet-backed)
	geojsonBytes, err := h.duckdbService.ExportChunkAsGeoJSON(chunkID)
	if err == nil {
		c.Data(http.StatusOK, "application/geo+json", geojsonBytes)
		return
	}

	log.Printf("failed to export chunk %s as geojson: %v; attempting fallback JSON read", chunkID, err)

	// Fallback: load chunk JSON and convert to GeoJSON (via service loadChunk -> ExportChunkAsGeoJSON already handles fallback)
	geojsonBytes, err = h.duckdbService.ExportChunkAsGeoJSON(chunkID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to export chunk as GeoJSON: " + err.Error()})
		return
	}

	c.Data(http.StatusOK, "application/geo+json", geojsonBytes)
}
