package routes

import (
	"time"

	"examle.com/mod/handlers"
	"examle.com/mod/services"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// SetupRoutes configures all API routes
func SetupRoutes(duckdbService *services.DuckDBService) *gin.Engine {
	r := gin.Default()

	// Configure CORS middleware
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:3000", "http://localhost:5173", "http://localhost:5174", "http://localhost:8080"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	restaurantHandler := handlers.NewRestaurantHandler(duckdbService)

	r.GET("/health", restaurantHandler.Health)

	v1 := r.Group("/api/v1")
	{
		v1.POST("/search", restaurantHandler.Search)
		v1.POST("/search-real", restaurantHandler.SearchReal)
		v1.POST("/preprocess", restaurantHandler.PreprocessChunk)
		v1.GET("/chunks", restaurantHandler.GetAvailableChunks)
		v1.POST("/search-chunk/:chunkId", restaurantHandler.SearchChunk)
		v1.GET("/chunks/:chunkId/geojson", restaurantHandler.GetChunkGeoJSON)
	}

	return r
}
