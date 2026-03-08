package services

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"examle.com/mod/models"
	_ "github.com/duckdb/duckdb-go/v2"
)

// DuckDBService provides methods to interact with DuckDB and Overture dataset
type DuckDBService struct {
	db                 *sql.DB
	spatialInitialized bool
	chunksDir          string
}

// NewDuckDBService creates a new DuckDB service
func NewDuckDBService() (*DuckDBService, error) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		return nil, fmt.Errorf("failed to open DuckDB connection: %w", err)
	}

	// Create chunks directory
	chunksDir := "chunks"
	if err := os.MkdirAll(chunksDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create chunks directory: %w", err)
	}

	service := &DuckDBService{
		db:        db,
		chunksDir: chunksDir,
	}

	// Initialize the database extensions
	if err := service.initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize DuckDB service: %w", err)
	}

	return service, nil
}

// Close closes the database connection
func (d *DuckDBService) Close() error {
	return d.db.Close()
}

// initialize sets up the DuckDB extensions and configurations
func (d *DuckDBService) initialize() error {
	// For sample data mode, we don't need spatial extension
	log.Println("DuckDB service initialized (sample data mode)")
	return nil
}

// initializeSpatial sets up the spatial extension for real Overture queries
func (d *DuckDBService) initializeSpatial() error {
	// Skip if already initialized
	if d.spatialInitialized {
		log.Println("✅ Spatial extension already initialized, skipping...")
		return nil
	}

	log.Println("🔧 Setting up spatial extension for Overture dataset access...")

	// Install spatial extension
	log.Println("📦 Installing spatial extension...")
	startTime := time.Now()
	_, err := d.db.Exec("INSTALL spatial")
	if err != nil {
		log.Printf("⚠️  Warning: Failed to install spatial extension (may already be installed): %v", err)
		log.Println("   This is usually fine if the extension was previously installed")
	} else {
		installDuration := time.Since(startTime)
		log.Printf("✅ Spatial extension installed successfully in %.2f seconds", installDuration.Seconds())
	}

	// Load spatial extension
	log.Println("🔄 Loading spatial extension...")
	loadStart := time.Now()
	_, err = d.db.Exec("LOAD spatial")
	if err != nil {
		log.Printf("❌ Failed to load spatial extension: %v", err)
		return fmt.Errorf("failed to load spatial extension: %w", err)
	}
	loadDuration := time.Since(loadStart)
	log.Printf("✅ Spatial extension loaded successfully in %.2f seconds", loadDuration.Seconds())

	// Set S3 region
	log.Println("🌎 Configuring S3 region for Overture dataset...")
	regionStart := time.Now()
	_, err = d.db.Exec("SET s3_region='us-west-2'")
	if err != nil {
		log.Printf("❌ Failed to set S3 region: %v", err)
		return fmt.Errorf("failed to set S3 region: %w", err)
	}
	regionDuration := time.Since(regionStart)
	log.Printf("✅ S3 region set to us-west-2 in %.2f seconds", regionDuration.Seconds())

	totalSetupTime := time.Since(startTime)
	log.Printf("🎉 Spatial setup completed in %.2f seconds total", totalSetupTime.Seconds())

	d.spatialInitialized = true
	return nil
}

// SearchRestaurants searches for restaurants based on the given criteria
func (d *DuckDBService) SearchRestaurants(req models.SearchRequest) ([]models.Restaurant, error) {
	// Initialize spatial extension for real queries
	if err := d.initializeSpatial(); err != nil {
		return nil, fmt.Errorf("failed to initialize spatial extension: %w", err)
	}

	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 10 // Default to 10 results
	}

	// Optimized query with proper spatial and categorical filtering
	query := `
		SELECT
			id,
			names.primary as name,
			confidence,
			CAST(socials AS JSON) as socials,
			ST_AsText(geometry) as geometry
		FROM
				read_parquet('s3://overturemaps-us-west-2/release/2026-02-18.0/theme=places/type=place/*', filename=true, hive_partitioning=1)
		WHERE
			categories.primary = $1
			AND bbox.xmin >= $2 AND bbox.xmax <= $3
			AND bbox.ymin >= $4 AND bbox.ymax <= $5
			AND ST_Within(
				geometry,
				ST_MakeEnvelope($2, $4, $3, $5, 4326)
			)
		ORDER BY confidence DESC
		LIMIT $6`

	rows, err := d.db.Query(query, req.Category, req.BboxMinX, req.BboxMaxX, req.BboxMinY, req.BboxMaxY, limit)
	if err != nil {
		// If the optimized query fails, fall back to a simpler but still efficient query
		log.Printf("Optimized query failed, trying fallback: %v", err)
		return d.searchRestaurantsFallback(req)
	}
	defer rows.Close()

	var restaurants []models.Restaurant
	for rows.Next() {
		var r models.Restaurant
		var socialsStr sql.NullString

		err := rows.Scan(&r.ID, &r.Name, &r.Confidence, &socialsStr, &r.Geometry)
		if err != nil {
			return nil, fmt.Errorf("failed to scan restaurant row: %w", err)
		}

		if socialsStr.Valid {
			r.Socials = json.RawMessage(socialsStr.String)
		} else {
			r.Socials = json.RawMessage("{}")
		}

		restaurants = append(restaurants, r)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over restaurant rows: %w", err)
	}

	return restaurants, nil
}

// searchRestaurantsFallback uses a simpler but more reliable query
func (d *DuckDBService) searchRestaurantsFallback(req models.SearchRequest) ([]models.Restaurant, error) {
	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	// Fallback query with better bbox filtering and no complex spatial functions
	query := `
		SELECT
			id,
			names.primary as name,
			confidence,
			CAST(socials AS JSON) as socials,
			ST_AsText(geometry) as geometry
		FROM (
			SELECT * FROM read_parquet('s3://overturemaps-us-west-2/release/2026-02-18.0/theme=places/type=place/*',
				filename=true, hive_partitioning=1)
			WHERE categories.primary = $1
				AND bbox.xmin BETWEEN $2 - 0.1 AND $3 + 0.1
				AND bbox.ymin BETWEEN $4 - 0.1 AND $5 + 0.1
				AND bbox.xmax BETWEEN $2 - 0.1 AND $3 + 0.1  
				AND bbox.ymax BETWEEN $4 - 0.1 AND $5 + 0.1
		)
		WHERE ST_Intersects(
			geometry,
			ST_GeomFromText('POLYGON((' || $2 || ' ' || $4 || ',' || $3 || ' ' || $4 || ',' || $3 || ' ' || $5 || ',' || $2 || ' ' || $5 || ',' || $2 || ' ' || $4 || '))', 4326)
		)
		ORDER BY confidence DESC
		LIMIT $6`

	rows, err := d.db.Query(query, req.Category, req.BboxMinX, req.BboxMaxX, req.BboxMinY, req.BboxMaxY, limit)
	if err != nil {
		return nil, fmt.Errorf("fallback query failed: %w", err)
	}
	defer rows.Close()

	var restaurants []models.Restaurant
	for rows.Next() {
		var r models.Restaurant
		var socialsStr sql.NullString

		err := rows.Scan(&r.ID, &r.Name, &r.Confidence, &socialsStr, &r.Geometry)
		if err != nil {
			return nil, fmt.Errorf("failed to scan restaurant row: %w", err)
		}

		if socialsStr.Valid {
			r.Socials = json.RawMessage(socialsStr.String)
		} else {
			r.Socials = json.RawMessage("{}")
		}

		restaurants = append(restaurants, r)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over restaurant rows: %w", err)
	}

	return restaurants, nil
}

// SearchRestaurantsUltraFast uses the most optimized query possible
func (d *DuckDBService) SearchRestaurantsUltraFast(req models.SearchRequest) ([]models.Restaurant, error) {
	// Initialize spatial extension for real queries
	if err := d.initializeSpatial(); err != nil {
		return nil, fmt.Errorf("failed to initialize spatial extension: %w", err)
	}

	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	// Ultra-optimized query using specific partitions and better filtering
	query := `
		SELECT
			id,
			names.primary as name,
			confidence,
			CAST(socials AS JSON) as socials,
			ST_AsText(geometry) as geometry
		FROM
			read_parquet('s3://overturemaps-us-west-2/release/2026-02-18.0/theme=places/type=place/*',
				filename=true, 
				hive_partitioning=1,
				file_row_number=true
			)
		WHERE
			categories.primary = $1
			AND (
				(bbox.xmin BETWEEN $2 AND $3) OR 
				(bbox.xmax BETWEEN $2 AND $3) OR
				(bbox.xmin <= $2 AND bbox.xmax >= $3)
			)
			AND (
				(bbox.ymin BETWEEN $4 AND $5) OR 
				(bbox.ymax BETWEEN $4 AND $5) OR
				(bbox.ymin <= $4 AND bbox.ymax >= $5)
			)
			AND confidence > 0.5
		ORDER BY 
			confidence DESC,
			CASE WHEN names.primary IS NOT NULL THEN 0 ELSE 1 END
		LIMIT $6`

	rows, err := d.db.Query(query, req.Category, req.BboxMinX, req.BboxMaxX, req.BboxMinY, req.BboxMaxY, limit)
	if err != nil {
		return nil, fmt.Errorf("ultra-fast query failed: %w", err)
	}
	defer rows.Close()

	var restaurants []models.Restaurant
	for rows.Next() {
		var r models.Restaurant
		var socialsStr sql.NullString

		err := rows.Scan(&r.ID, &r.Name, &r.Confidence, &socialsStr, &r.Geometry)
		if err != nil {
			return nil, fmt.Errorf("failed to scan restaurant row: %w", err)
		}

		if socialsStr.Valid {
			r.Socials = json.RawMessage(socialsStr.String)
		} else {
			r.Socials = json.RawMessage("{}")
		}

		restaurants = append(restaurants, r)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over restaurant rows: %w", err)
	}

	return restaurants, nil
}
func (d *DuckDBService) GetSamplePizzaRestaurants() []models.Restaurant {
	return []models.Restaurant{
		{
			ID:         "sample_1",
			Name:       "Mario's Pizza",
			Confidence: 0.95,
			Socials:    json.RawMessage(`{"website": "https://mariospizza.com"}`),
			Geometry:   "POINT(-74.0059 40.7128)",
		},
		{
			ID:         "sample_2",
			Name:       "Tony's Italian",
			Confidence: 0.87,
			Socials:    json.RawMessage(`{}`),
			Geometry:   "POINT(-74.0059 40.7589)",
		},
		{
			ID:         "sample_3",
			Name:       "Brooklyn Pizza Co",
			Confidence: 0.92,
			Socials:    json.RawMessage(`{"facebook": "brooklynpizzaco"}`),
			Geometry:   "POINT(-73.9442 40.6782)",
		},
		{
			ID:         "sample_4",
			Name:       "Authentic NY Slice",
			Confidence: 0.89,
			Socials:    json.RawMessage(`{"instagram": "@authenticnyslice"}`),
			Geometry:   "POINT(-73.9857 40.7484)",
		},
		{
			ID:         "sample_5",
			Name:       "Little Italy Pizza",
			Confidence: 0.91,
			Socials:    json.RawMessage(`{}`),
			Geometry:   "POINT(-73.9973 40.7589)",
		},
		{
			ID:         "sample_6",
			Name:       "Giuseppe's Pizzeria",
			Confidence: 0.88,
			Socials:    json.RawMessage(`{"website": "https://giuseppes.com", "phone": "555-0123"}`),
			Geometry:   "POINT(-74.0021 40.7505)",
		},
		{
			ID:         "sample_7",
			Name:       "Corner Pizza Shop",
			Confidence: 0.85,
			Socials:    json.RawMessage(`{}`),
			Geometry:   "POINT(-73.9776 40.7639)",
		},
		{
			ID:         "sample_8",
			Name:       "Mama Mia's",
			Confidence: 0.93,
			Socials:    json.RawMessage(`{"facebook": "mamamiasnyc", "instagram": "@mamamiasny"}`),
			Geometry:   "POINT(-73.9442 40.8176)",
		},
		{
			ID:         "sample_9",
			Name:       "Pizza Palace",
			Confidence: 0.86,
			Socials:    json.RawMessage(`{"website": "https://pizzapalace.nyc"}`),
			Geometry:   "POINT(-73.9857 40.7282)",
		},
		{
			ID:         "sample_10",
			Name:       "Napoli Pizza House",
			Confidence: 0.90,
			Socials:    json.RawMessage(`{"phone": "555-0456", "website": "https://napolipizzahouse.com"}`),
			Geometry:   "POINT(-74.0059 40.7831)",
		},
	}
}

// PreprocessChunk preprocesses a geographic chunk and saves it locally
func (d *DuckDBService) PreprocessChunk(req models.PreprocessRequest) (*models.PreprocessResponse, error) {
	log.Printf("🚀 Starting preprocessing for category: %s, bbox: [%.6f,%.6f,%.6f,%.6f]",
		req.Category, req.BboxMinX, req.BboxMinY, req.BboxMaxX, req.BboxMaxY)

	// Calculate area and provide suggestions for large areas
	area := (req.BboxMaxX - req.BboxMinX) * (req.BboxMaxY - req.BboxMinY)
	log.Printf("📏 Bounding box area: %.6f degrees²", area)

	if area > 0.5 {
		log.Printf("⚠️  WARNING: Large area detected (%.2f degrees²)", area)
		log.Printf("💡 For faster processing, consider breaking this into smaller chunks:")

		// Suggest smaller bounding boxes
		suggestions := d.suggestSmallerBboxes(req.BboxMinX, req.BboxMinY, req.BboxMaxX, req.BboxMaxY)
		for i, bbox := range suggestions {
			log.Printf("   Chunk %d: [%.6f, %.6f, %.6f, %.6f]", i+1, bbox[0], bbox[1], bbox[2], bbox[3])
		}

		if area > 1.0 {
			return nil, fmt.Errorf("bounding box too large (%.2f degrees²). Please use areas smaller than 1.0 degrees² for reasonable performance. See suggested chunks in logs above", area)
		}

		log.Printf("⏳ Proceeding with large area - this will take several minutes...")
	}

	// Generate chunk ID if not provided
	chunkID := req.ChunkID
	if chunkID == "" {
		chunkID = d.generateChunkID(req)
		log.Printf("📋 Generated chunk ID: %s", chunkID)
	} else {
		log.Printf("📋 Using provided chunk ID: %s", chunkID)
	}

	// Initialize spatial extension for real data processing
	log.Println("🔧 Initializing spatial extension...")
	if err := d.initializeSpatial(); err != nil {
		log.Printf("❌ Failed to initialize spatial extension: %v", err)
		return nil, fmt.Errorf("failed to initialize spatial extension: %w", err)
	}
	log.Println("✅ Spatial extension initialized successfully")

	// Query to extract data for the specified chunk - OPTIMIZED for speed
	log.Println("📊 Preparing optimized query for Overture dataset...")

	// Calculate area to determine if we need to break this into smaller chunks
	area = (req.BboxMaxX - req.BboxMinX) * (req.BboxMaxY - req.BboxMinY)
	log.Printf("📏 Bounding box area: %.6f degrees² (larger areas take longer)", area)

	if area > 1.0 {
		log.Printf("⚠️  Large area detected! This will take significant time.")
		log.Printf("💡 Consider using smaller bounding boxes (< 1.0 degrees²) for faster results")
	}

	// Use a more efficient query with better filtering
	query := `
		SELECT 
			id,
			names.primary as name,
			confidence,
			socials,
			ST_AsText(geometry) as geometry
		FROM 
			read_parquet('s3://overturemaps-us-west-2/release/2026-02-18.0/theme=places/type=place/*', 
						 hive_partitioning=1,
						 filename=true) 
		WHERE 
			categories.primary = ? 
			AND confidence > 0.7
			AND names.primary IS NOT NULL
			AND bbox.xmin >= ? AND bbox.xmin <= ?
			AND bbox.ymin >= ? AND bbox.ymin <= ?
			AND (bbox.xmax - bbox.xmin) < 0.1
			AND (bbox.ymax - bbox.ymin) < 0.1
		ORDER BY confidence DESC
		LIMIT 1000
	`

	log.Printf("🔍 Executing optimized query against Overture dataset...")
	log.Printf("   Category: %s (filtered for confidence > 0.7)", req.Category)
	log.Printf("   Bounding box: xmin=%.6f, xmax=%.6f, ymin=%.6f, ymax=%.6f",
		req.BboxMinX, req.BboxMaxX, req.BboxMinY, req.BboxMaxY)
	log.Printf("   Limit: 1000 records (prevents excessive data transfer)")

	// Start a progress monitor goroutine with timeout
	done := make(chan bool)
	timeout := make(chan bool)

	go func() {
		ticker := time.NewTicker(5 * time.Second) // Check every 5 seconds
		defer ticker.Stop()
		elapsed := 0
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				elapsed += 5
				log.Printf("⏱️  Query still running... %d seconds elapsed", elapsed)
				if elapsed == 30 {
					log.Printf("   Large geographic areas can take 1-2 minutes")
				}
				if elapsed == 60 {
					log.Printf("   Still processing... Consider using smaller bounding boxes")
				}
				if elapsed >= 120 {
					log.Printf("   Query taking very long, but this can happen with large areas")
				}
				if elapsed >= 300 { // 5 minutes timeout
					log.Printf("❌ Query timeout after 5 minutes - area too large")
					timeout <- true
					return
				}
			}
		}
	}()

	startTime := time.Now()

	// Create a channel to receive query results
	resultChan := make(chan struct {
		rows *sql.Rows
		err  error
	}, 1)

	// Run query in goroutine
	go func() {
		rows, err := d.db.Query(query, req.Category, req.BboxMinX, req.BboxMaxX, req.BboxMinY, req.BboxMaxY)
		resultChan <- struct {
			rows *sql.Rows
			err  error
		}{rows, err}
	}()

	// Wait for either query completion or timeout
	var rows *sql.Rows
	var err error

	select {
	case result := <-resultChan:
		rows, err = result.rows, result.err
		done <- true // Stop the progress monitor
	case <-timeout:
		return nil, fmt.Errorf("query timeout after 5 minutes - bounding box area too large (%.6f degrees²). Try using smaller areas", area)
	}

	if err != nil {
		log.Printf("❌ Query failed after %.2f seconds: %v", time.Since(startTime).Seconds(), err)
		return nil, fmt.Errorf("failed to query chunk data: %w", err)
	}
	defer rows.Close()

	queryDuration := time.Since(startTime)
	log.Printf("✅ Query completed in %.2f seconds, processing results...", queryDuration.Seconds())

	var restaurants []models.Restaurant
	rowCount := 0
	processingStart := time.Now()

	for rows.Next() {
		var restaurant models.Restaurant
		var socialsStr string

		err := rows.Scan(
			&restaurant.ID,
			&restaurant.Name,
			&restaurant.Confidence,
			&socialsStr,
			&restaurant.Geometry,
		)
		if err != nil {
			log.Printf("⚠️  Error scanning row %d: %v", rowCount+1, err)
			continue
		}

		// Parse socials JSON
		if socialsStr != "" {
			restaurant.Socials = json.RawMessage(socialsStr)
		} else {
			restaurant.Socials = json.RawMessage("{}")
		}

		restaurants = append(restaurants, restaurant)
		rowCount++

		// Log progress every 100 records
		if rowCount%100 == 0 {
			log.Printf("📈 Processed %d records so far...", rowCount)
		}
	}

	processingDuration := time.Since(processingStart)
	log.Printf("✅ Finished processing %d records in %.2f seconds", len(restaurants), processingDuration.Seconds())

	if len(restaurants) == 0 {
		log.Printf("⚠️  No restaurants found for the specified criteria")
		log.Printf("   This might mean:")
		log.Printf("   - No data exists in this bounding box")
		log.Printf("   - Category '%s' doesn't match any records", req.Category)
		log.Printf("   - Bounding box coordinates might be incorrect")
	}

	// Save chunk to local file
	log.Printf("💾 Saving chunk data to local storage...")
	saveStart := time.Now()
	if err := d.saveChunk(chunkID, req, restaurants); err != nil {
		log.Printf("❌ Failed to save chunk after %.2f seconds: %v", time.Since(saveStart).Seconds(), err)
		return nil, fmt.Errorf("failed to save chunk: %w", err)
	}
	saveDuration := time.Since(saveStart)
	log.Printf("✅ Chunk saved successfully in %.2f seconds", saveDuration.Seconds())

	totalDuration := time.Since(startTime)
	log.Printf("🎉 Preprocessing completed successfully!")
	log.Printf("   Total time: %.2f seconds", totalDuration.Seconds())
	log.Printf("   Query time: %.2f seconds", queryDuration.Seconds())
	log.Printf("   Processing time: %.2f seconds", processingDuration.Seconds())
	log.Printf("   Save time: %.2f seconds", saveDuration.Seconds())
	log.Printf("   Records found: %d", len(restaurants))
	log.Printf("   Chunk ID: %s", chunkID)

	return &models.PreprocessResponse{
		ChunkID:     chunkID,
		Category:    req.Category,
		BboxMinX:    req.BboxMinX,
		BboxMaxX:    req.BboxMaxX,
		BboxMinY:    req.BboxMinY,
		BboxMaxY:    req.BboxMaxY,
		RecordCount: len(restaurants),
		Message:     fmt.Sprintf("Preprocessed %d records for chunk %s in %.2f seconds", len(restaurants), chunkID, totalDuration.Seconds()),
	}, nil
}

// GetAvailableChunks returns information about all precomputed chunks
func (d *DuckDBService) GetAvailableChunks() (*models.ChunksListResponse, error) {
	var chunks []models.ChunkInfo

	// Read chunks directory
	files, err := os.ReadDir(d.chunksDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read chunks directory: %w", err)
	}

	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		chunkID := strings.TrimSuffix(file.Name(), ".json")
		chunkPath := filepath.Join(d.chunksDir, file.Name())

		// Get file info for creation time
		fileInfo, err := file.Info()
		if err != nil {
			log.Printf("Failed to get file info for %s: %v", file.Name(), err)
			continue
		}

		// Load chunk metadata
		_, err = d.loadChunk(chunkID)
		if err != nil {
			log.Printf("Failed to load chunk %s: %v", chunkID, err)
			continue
		}

		// Extract metadata from first entry (assuming metadata is stored with data)
		var chunkMeta struct {
			Category    string              `json:"category"`
			BboxMinX    float64             `json:"bbox_min_x"`
			BboxMaxX    float64             `json:"bbox_max_x"`
			BboxMinY    float64             `json:"bbox_min_y"`
			BboxMaxY    float64             `json:"bbox_max_y"`
			Restaurants []models.Restaurant `json:"restaurants"`
		}

		chunkFile, err := os.ReadFile(chunkPath)
		if err != nil {
			log.Printf("Failed to read chunk file %s: %v", chunkPath, err)
			continue
		}

		if err := json.Unmarshal(chunkFile, &chunkMeta); err != nil {
			log.Printf("Failed to unmarshal chunk metadata %s: %v", chunkID, err)
			continue
		}

		chunks = append(chunks, models.ChunkInfo{
			ChunkID:     chunkID,
			Category:    chunkMeta.Category,
			BboxMinX:    chunkMeta.BboxMinX,
			BboxMaxX:    chunkMeta.BboxMaxX,
			BboxMinY:    chunkMeta.BboxMinY,
			BboxMaxY:    chunkMeta.BboxMaxY,
			RecordCount: len(chunkMeta.Restaurants),
			CreatedAt:   fileInfo.ModTime().Format(time.RFC3339),
		})
	}

	return &models.ChunksListResponse{
		Chunks:  chunks,
		Total:   len(chunks),
		Message: fmt.Sprintf("Found %d precomputed chunks", len(chunks)),
	}, nil
}

// SearchFromChunk searches within a precomputed chunk
func (d *DuckDBService) SearchFromChunk(chunkID string, limit int) ([]models.Restaurant, error) {
	chunkData, err := d.loadChunk(chunkID)
	if err != nil {
		return nil, fmt.Errorf("failed to load chunk %s: %w", chunkID, err)
	}

	// Apply limit
	restaurants := chunkData.Restaurants
	if limit > 0 && len(restaurants) > limit {
		restaurants = restaurants[:limit]
	}

	return restaurants, nil
}

// generateChunkID creates a unique identifier for a chunk based on its parameters
func (d *DuckDBService) generateChunkID(req models.PreprocessRequest) string {
	data := fmt.Sprintf("%s_%.6f_%.6f_%.6f_%.6f",
		req.Category, req.BboxMinX, req.BboxMaxX, req.BboxMinY, req.BboxMaxY)

	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)[:16] // Use first 16 characters of hash
}

// saveChunk saves chunk data to a local file
func (d *DuckDBService) saveChunk(chunkID string, req models.PreprocessRequest, restaurants []models.Restaurant) error {
	chunkData := struct {
		ChunkID     string              `json:"chunk_id"`
		Category    string              `json:"category"`
		BboxMinX    float64             `json:"bbox_min_x"`
		BboxMaxX    float64             `json:"bbox_max_x"`
		BboxMinY    float64             `json:"bbox_min_y"`
		BboxMaxY    float64             `json:"bbox_max_y"`
		CreatedAt   string              `json:"created_at"`
		Restaurants []models.Restaurant `json:"restaurants"`
	}{
		ChunkID:     chunkID,
		Category:    req.Category,
		BboxMinX:    req.BboxMinX,
		BboxMaxX:    req.BboxMaxX,
		BboxMinY:    req.BboxMinY,
		BboxMaxY:    req.BboxMaxY,
		CreatedAt:   time.Now().Format(time.RFC3339),
		Restaurants: restaurants,
	}

	chunkPath := filepath.Join(d.chunksDir, chunkID+".json")
	data, err := json.MarshalIndent(chunkData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal chunk data: %w", err)
	}

	return os.WriteFile(chunkPath, data, 0644)
}

// loadChunk loads chunk data from a local file
func (d *DuckDBService) loadChunk(chunkID string) (*struct {
	ChunkID     string              `json:"chunk_id"`
	Category    string              `json:"category"`
	BboxMinX    float64             `json:"bbox_min_x"`
	BboxMaxX    float64             `json:"bbox_max_x"`
	BboxMinY    float64             `json:"bbox_min_y"`
	BboxMaxY    float64             `json:"bbox_max_y"`
	CreatedAt   string              `json:"created_at"`
	Restaurants []models.Restaurant `json:"restaurants"`
}, error) {
	chunkPath := filepath.Join(d.chunksDir, chunkID+".json")

	data, err := os.ReadFile(chunkPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read chunk file: %w", err)
	}

	var chunkData struct {
		ChunkID     string              `json:"chunk_id"`
		Category    string              `json:"category"`
		BboxMinX    float64             `json:"bbox_min_x"`
		BboxMaxX    float64             `json:"bbox_max_x"`
		BboxMinY    float64             `json:"bbox_min_y"`
		BboxMaxY    float64             `json:"bbox_max_y"`
		CreatedAt   string              `json:"created_at"`
		Restaurants []models.Restaurant `json:"restaurants"`
	}

	if err := json.Unmarshal(data, &chunkData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal chunk data: %w", err)
	}

	return &chunkData, nil
}

// suggestSmallerBboxes breaks a large bounding box into smaller, more manageable chunks
func (d *DuckDBService) suggestSmallerBboxes(minX, minY, maxX, maxY float64) [][]float64 {
	var suggestions [][]float64

	// Target chunk size of ~0.25 degrees²
	targetSize := 0.25

	xWidth := maxX - minX
	yHeight := maxY - minY

	// Calculate how many divisions we need
	xDivisions := int(xWidth/math.Sqrt(targetSize)) + 1
	yDivisions := int(yHeight/math.Sqrt(targetSize)) + 1

	// Ensure we don't create too many chunks
	if xDivisions > 4 {
		xDivisions = 4
	}
	if yDivisions > 4 {
		yDivisions = 4
	}

	xStep := xWidth / float64(xDivisions)
	yStep := yHeight / float64(yDivisions)

	for i := 0; i < xDivisions; i++ {
		for j := 0; j < yDivisions; j++ {
			chunkMinX := minX + float64(i)*xStep
			chunkMinY := minY + float64(j)*yStep
			chunkMaxX := minX + float64(i+1)*xStep
			chunkMaxY := minY + float64(j+1)*yStep

			suggestions = append(suggestions, []float64{chunkMinX, chunkMinY, chunkMaxX, chunkMaxY})
		}
	}

	return suggestions
}
