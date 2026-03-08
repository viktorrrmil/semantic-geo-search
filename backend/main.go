package main

import (
	"log"

	"examle.com/mod/routes"
	"examle.com/mod/services"
)

func main() {
	// Initialize DuckDB service
	duckdbService, err := services.NewDuckDBService()
	if err != nil {
		log.Fatal("Failed to initialize DuckDB service:", err)
	}
	defer duckdbService.Close()

	// Setup routes
	router := routes.SetupRoutes(duckdbService)

	// Start server
	log.Println("Starting Semantic Geo Search API on :3001")
	log.Println("Available endpoints:")
	log.Println("  GET  /health")
	log.Println("  POST /api/v1/search         - Fast sample data")
	log.Println("  POST /api/v1/search-real    - Real Overture data (optimized)")
	log.Println("  POST /api/v1/preprocess     - Preprocess and cache a geographic chunk")
	log.Println("  GET  /api/v1/chunks         - List all precomputed chunks")
	log.Println("  POST /api/v1/search-chunk/:id - Search within a precomputed chunk")
	log.Println()
	log.Println("Example search request:")
	log.Println(`curl -X POST http://localhost:3001/api/v1/search \
  -H "Content-Type: application/json" \
  -d '{
    "category": "pizza_restaurant",
    "bbox_min_x": -75.0,
    "bbox_max_x": -73.0,
    "bbox_min_y": 40.0,
    "bbox_max_y": 41.0,
    "limit": 10
  }'`)
	log.Println()
	log.Println("Example preprocess request:")
	log.Println(`curl -X POST http://localhost:3001/api/v1/preprocess \
  -H "Content-Type: application/json" \
  -d '{
    "category": "pizza_restaurant",
    "bbox_min_x": -73.99,
    "bbox_max_x": -73.95,
    "bbox_min_y": 40.75,
    "bbox_max_y": 40.78
  }'`)

	if err := router.Run(":3001"); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}
