package services

import (
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
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
	sourceparquet      string // configurable source parquet path
}

// NewDuckDBService creates a new DuckDB service
func NewDuckDBService(sourceParquet string) (*DuckDBService, error) {
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
		db:            db,
		chunksDir:     chunksDir,
		sourceparquet: sourceParquet,
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
	return nil
}

// initializeSpatial sets up the spatial extension for real Overture queries
func (d *DuckDBService) initializeSpatial() error {
	if d.spatialInitialized {
		return nil
	}

	_, _ = d.db.Exec("INSTALL httpfs")
	_, _ = d.db.Exec("LOAD httpfs")
	_, _ = d.db.Exec("SET s3_region='us-west-2'")
	_, _ = d.db.Exec("SET threads=4")
	_, _ = d.db.Exec("SET memory_limit='2GB'")

	_, err := d.db.Exec("INSTALL spatial")

	// Load spatial extension
	_, err = d.db.Exec("LOAD spatial")
	if err != nil {
		return fmt.Errorf("failed to load spatial extension: %w", err)
	}

	_, err = d.db.Exec("SET s3_region='us-west-2'")
	if err != nil {
		return fmt.Errorf("failed to set S3 region: %w", err)
	}

	d.spatialInitialized = true
	return nil
}

// SearchRestaurants searches for restaurants based on the given criteria
func (d *DuckDBService) SearchRestaurants(req models.SearchRequest) ([]models.Restaurant, error) {
	if err := d.initializeSpatial(); err != nil {
		return nil, fmt.Errorf("failed to initialize spatial extension: %w", err)
	}

	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	// Use configured source parquet path (d.sourceparquet) instead of hardcoded filename
	query := fmt.Sprintf(`
		SELECT
			id,
			names.primary as name,
			confidence,
			CAST(TO_JSON(socials) AS VARCHAR) as socials,
			ST_AsText(TRY_CAST(geometry AS GEOMETRY)) as geometry
		FROM
			read_parquet('%s')
		WHERE
			categories.primary = ?
			AND bbox.xmin BETWEEN ? AND ?
			AND bbox.ymin BETWEEN ? AND ?
		LIMIT ?`, d.sourceparquet)

	rows, err := d.db.Query(query, req.Category, req.BboxMinX, req.BboxMaxX, req.BboxMinY, req.BboxMaxY, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query restaurants: %w", err)
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

	// Use a more efficient query with better filtering
	var query string
	var queryArgs []interface{}
	if strings.TrimSpace(req.Category) == "" {
		// No category: preprocess all places within bbox; use configured source parquet
		query = fmt.Sprintf(`
		SELECT 
		id,
		names.primary as name,
		confidence,
		CAST(TO_JSON(socials) AS VARCHAR) as socials,
		ST_AsText(TRY_CAST(geometry AS GEOMETRY)) as geometry
		FROM 
		read_parquet('%s')
		WHERE  
		confidence > 0.7
		AND names.primary IS NOT NULL
		AND bbox.xmin >= ? AND bbox.xmax <= ?
		AND bbox.ymin >= ? AND bbox.ymax <= ?
		ORDER BY confidence DESC
		LIMIT 1000
		`, d.sourceparquet)
		queryArgs = []interface{}{req.BboxMinX, req.BboxMaxX, req.BboxMinY, req.BboxMaxY}
	} else {
		query = fmt.Sprintf(`
		SELECT 
		id,
		names.primary as name,
		confidence,
		CAST(TO_JSON(socials) AS VARCHAR) as socials,
		ST_AsText(TRY_CAST(geometry AS GEOMETRY)) as geometry
		FROM 
		read_parquet('%s')
		WHERE 
		categories.primary = ? 
		AND confidence > 0.7
		AND names.primary IS NOT NULL
		AND bbox.xmin >= ? AND bbox.xmax <= ?
		AND bbox.ymin >= ? AND bbox.ymax <= ?
		ORDER BY confidence DESC
		LIMIT 1000
		`, d.sourceparquet)
		queryArgs = []interface{}{req.Category, req.BboxMinX, req.BboxMaxX, req.BboxMinY, req.BboxMaxY}
	}

	log.Printf("🔍 Executing optimized query against Overture dataset...")
	log.Printf("   Category: %s", req.Category)
	log.Printf("   Bounding box: xmin=%.6f, xmax=%.6f, ymin=%.6f, ymax=%.6f",
		req.BboxMinX, req.BboxMaxX, req.BboxMinY, req.BboxMaxY)
	log.Printf("   Limit: 1000 records (prevents excessive data transfer)")

	// Start a progress monitor goroutine with timeout (monitor only — timeout handled in wait below)
	done := make(chan bool)

	go func() {
		ticker := time.NewTicker(10 * time.Second) // Check every 10 seconds
		defer ticker.Stop()
		elapsed := 0
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				elapsed += 10
				log.Printf("⏱️  Query still running... %d seconds elapsed", elapsed)
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
		rows, err := d.db.Query(query, queryArgs...)
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
	case <-time.After(10 * time.Minute):
		return nil, fmt.Errorf("query timeout after 10 minutes - bounding box area too large (%.6f degrees²). Try using smaller areas", area)
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
		var socialsStr sql.NullString

		err := rows.Scan(&restaurant.ID, &restaurant.Name, &restaurant.Confidence, &socialsStr, &restaurant.Geometry)

		if err != nil {
			log.Printf("⚠️  Error scanning row %d: %v", rowCount+1, err)
			continue
		}

		// Parse socials JSON
		if socialsStr.Valid && socialsStr.String != "" {
			restaurant.Socials = json.RawMessage(socialsStr.String)
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
		log.Printf("⚠️  No places found for the specified criteria")
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

// GetChunkMetadata reads the chunk JSON metadata (always reads the JSON file written by saveChunk)
// and returns the parsed metadata. This avoids relying on parquet reads to obtain bbox/category.
func (d *DuckDBService) GetChunkMetadata(chunkID string) (*struct {
	ChunkID   string  `json:"chunk_id"`
	Category  string  `json:"category"`
	BboxMinX  float64 `json:"bbox_min_x"`
	BboxMaxX  float64 `json:"bbox_max_x"`
	BboxMinY  float64 `json:"bbox_min_y"`
	BboxMaxY  float64 `json:"bbox_max_y"`
	CreatedAt string  `json:"created_at"`
}, error) {
	jsonPath := filepath.Join(d.chunksDir, chunkID+".json")
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read chunk metadata json: %w", err)
	}

	var meta struct {
		ChunkID   string  `json:"chunk_id"`
		Category  string  `json:"category"`
		BboxMinX  float64 `json:"bbox_min_x"`
		BboxMaxX  float64 `json:"bbox_max_x"`
		BboxMinY  float64 `json:"bbox_min_y"`
		BboxMaxY  float64 `json:"bbox_max_y"`
		CreatedAt string  `json:"created_at"`
	}

	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("failed to unmarshal chunk metadata json: %w", err)
	}

	return &meta, nil
}

// SearchParquetChunk executes a spatial search directly against a local parquet chunk for best performance.
// It requires a parquet file to exist (created by saveChunk). If parquet or spatial functions are unavailable
// it returns an error so callers can fallback to the in-memory JSON search.
func (d *DuckDBService) SearchParquetChunk(chunkID string, xmin, ymin, xmax, ymax float64, limit int) ([]models.Restaurant, error) {
	if limit <= 0 {
		limit = 10
	}

	parquetPath := filepath.Join(d.chunksDir, chunkID+".parquet")
	if _, err := os.Stat(parquetPath); err != nil {
		return nil, fmt.Errorf("parquet not found for chunk %s: %w", chunkID, err)
	}

	// Ensure spatial extension loaded
	if err := d.initializeSpatial(); err != nil {
		return nil, fmt.Errorf("failed to initialize spatial extension: %w", err)
	}

	absParquet, _ := filepath.Abs(parquetPath)
	// Use ST_MakeEnvelope for bbox intersection
	query := fmt.Sprintf("SELECT id, name, confidence, CAST(TO_JSON(socials) AS VARCHAR) as socials, ST_AsText(ST_GeomFromWKB(CAST(geometry AS BLOB))) as geometry FROM read_parquet('%s') WHERE ST_Intersects(ST_GeomFromWKB(CAST(geometry AS BLOB)), ST_MakeEnvelope(%f, %f, %f, %f)) ORDER BY confidence DESC LIMIT %d", absParquet, xmin, ymin, xmax, ymax, limit)

	rows, err := d.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query parquet chunk: %w", err)
	}
	defer rows.Close()

	var results []models.Restaurant
	for rows.Next() {
		var r models.Restaurant
		var socials sql.NullString
		if err := rows.Scan(&r.ID, &r.Name, &r.Confidence, &socials, &r.Geometry); err != nil {
			log.Printf("warning: failed to scan parquet search row: %v", err)
			continue
		}
		if socials.Valid {
			r.Socials = json.RawMessage(socials.String)
		} else {
			r.Socials = json.RawMessage("{}")
		}
		results = append(results, r)
	}

	if err := rows.Err(); err != nil {
		return results, fmt.Errorf("error iterating parquet rows: %w", err)
	}

	return results, nil
}

// generateChunkID creates a unique identifier for a chunk based on its parameters
func (d *DuckDBService) generateChunkID(req models.PreprocessRequest) string {
	var data string
	if strings.TrimSpace(req.Category) == "" {
		data = fmt.Sprintf("bbox_%.6f_%.6f_%.6f_%.6f",
			req.BboxMinX, req.BboxMaxX, req.BboxMinY, req.BboxMaxY)
	} else {
		data = fmt.Sprintf("%s_%.6f_%.6f_%.6f_%.6f",
			req.Category, req.BboxMinX, req.BboxMaxX, req.BboxMinY, req.BboxMaxY)
	}

	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)[:16] // Use first 16 characters of hash
}

// saveChunk saves chunk data to a local file and writes a GeoParquet for faster reads
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

	if err := os.WriteFile(chunkPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write chunk json file: %w", err)
	}

	// Also write a GeoParquet for faster reads. We'll create a temporary CSV and use DuckDB's COPY to write Parquet with geometry.
	csvPath := filepath.Join(d.chunksDir, chunkID+".csv")
	parquetPath := filepath.Join(d.chunksDir, chunkID+".parquet")

	csvFile, err := os.Create(csvPath)
	if err != nil {
		return fmt.Errorf("failed to create temp csv file: %w", err)
	}
	defer csvFile.Close()

	writer := csv.NewWriter(csvFile)
	// header
	if err := writer.Write([]string{"id", "name", "confidence", "socials", "geometry"}); err != nil {
		return fmt.Errorf("failed to write csv header: %w", err)
	}

	for _, r := range restaurants {
		// socials is json.RawMessage -> convert to string
		socialsStr := string(r.Socials)
		record := []string{r.ID, r.Name, fmt.Sprintf("%f", r.Confidence), socialsStr, r.Geometry}
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("failed to write csv record: %w", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("csv writer error: %w", err)
	}

	// Ensure spatial extension loaded for ST_GeomFromText
	if err := d.initializeSpatial(); err != nil {
		// Not fatal for saving parquet, but log and continue
		log.Printf("warning: failed to initialize spatial extension for parquet write: %v", err)
	}

	// Use DuckDB to convert CSV -> Parquet (geometry as geometry type)
	// COPY (SELECT id, name, confidence, socials, ST_GeomFromText(geometry) AS geometry FROM read_csv_auto('...')) TO '...' (FORMAT PARQUET)
	absCSV, _ := filepath.Abs(csvPath)
	absParquet, _ := filepath.Abs(parquetPath)
	copyQuery := fmt.Sprintf("COPY (SELECT id, name, confidence, socials, ST_GeomFromText(geometry) AS geometry FROM read_csv_auto('%s', header=TRUE)) TO '%s' (FORMAT PARQUET)", absCSV, absParquet)

	if _, err := d.db.Exec(copyQuery); err != nil {
		// If parquet write fails, log but return original error only if needed
		log.Printf("warning: failed to write parquet for chunk %s: %v", chunkID, err)
		// attempt cleanup of csv and continue
		_ = os.Remove(csvPath)
		return nil // still consider success because JSON was written
	}

	// remove tmp csv
	_ = os.Remove(csvPath)

	return nil
}

// loadChunk loads chunk data from a local parquet file if available, otherwise falls back to JSON
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
	parquetPath := filepath.Join(d.chunksDir, chunkID+".parquet")
	jsonPath := filepath.Join(d.chunksDir, chunkID+".json")

	// If parquet exists, read from it using DuckDB for faster, typed reads
	if _, err := os.Stat(parquetPath); err == nil {
		// Ensure spatial extension is loaded
		if err := d.initializeSpatial(); err != nil {
			log.Printf("warning: failed to initialize spatial extension when loading parquet: %v", err)
		}

		absParquet, _ := filepath.Abs(parquetPath)
		query := fmt.Sprintf("SELECT id, name, confidence, CAST(TO_JSON(socials) AS VARCHAR) as socials, ST_AsText(ST_GeomFromWKB(CAST(geometry AS BLOB))) as geometry FROM read_parquet('%s')\n", absParquet)
		rows, err := d.db.Query(query)
		if err != nil {
			log.Printf("failed to read parquet %s: %v", parquetPath, err)
			// Fall back to JSON below
		} else {
			defer rows.Close()
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

			chunkData.ChunkID = chunkID
			chunkData.Category = ""
			chunkData.CreatedAt = ""

			for rows.Next() {
				var r models.Restaurant
				var socialsStr sql.NullString
				if err := rows.Scan(&r.ID, &r.Name, &r.Confidence, &socialsStr, &r.Geometry); err != nil {
					log.Printf("warning: failed to scan parquet row: %v", err)
					continue
				}
				if socialsStr.Valid {
					r.Socials = json.RawMessage(socialsStr.String)
				} else {
					r.Socials = json.RawMessage("{}")
				}
				chunkData.Restaurants = append(chunkData.Restaurants, r)
			}

			if err := rows.Err(); err != nil {
				log.Printf("warning: error iterating parquet rows: %v", err)
			}

			return &chunkData, nil
		}
	}

	// Fallback to JSON file if parquet missing or failed
	data, err := os.ReadFile(jsonPath)
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

// ExportChunkAsGeoJSON converts a stored chunk into a GeoJSON FeatureCollection and returns the bytes.
// This prefers the parquet-backed fast path (via loadChunk) and marshals to GeoJSON in Go.
func (d *DuckDBService) ExportChunkAsGeoJSON(chunkID string) ([]byte, error) {
	chunkData, err := d.loadChunk(chunkID)
	if err != nil {
		return nil, fmt.Errorf("failed to load chunk %s: %w", chunkID, err)
	}

	// Build GeoJSON FeatureCollection
	type feature struct {
		Type       string                 `json:"type"`
		Properties map[string]interface{} `json:"properties"`
		Geometry   interface{}            `json:"geometry"`
	}

	fc := map[string]interface{}{
		"type":     "FeatureCollection",
		"features": make([]feature, 0, len(chunkData.Restaurants)),
	}

	for _, r := range chunkData.Restaurants {
		geom := parseWKTToGeoJSON(r.Geometry)
		props := map[string]interface{}{
			"id":         r.ID,
			"name":       r.Name,
			"confidence": r.Confidence,
		}
		// include socials if non-empty
		if len(r.Socials) > 0 {
			var socials interface{}
			if err := json.Unmarshal(r.Socials, &socials); err == nil {
				props["socials"] = socials
			} else {
				props["socials"] = string(r.Socials)
			}
		}

		f := feature{
			Type:       "Feature",
			Properties: props,
			Geometry:   geom,
		}
		fc["features"] = append(fc["features"].([]feature), f)
	}

	out, err := json.MarshalIndent(fc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal geojson: %w", err)
	}

	return out, nil
}

// parseWKTToGeoJSON supports a minimal WKT -> GeoJSON conversion for POINT geometries.
// Returns nil if geometry cannot be parsed.
func parseWKTToGeoJSON(wkt string) interface{} {
	w := strings.TrimSpace(wkt)
	if w == "" {
		return nil
	}
	// Keep original casing when extracting numbers; but check prefix case-insensitively
	upper := strings.ToUpper(w)
	if strings.HasPrefix(upper, "POINT(") && strings.HasSuffix(upper, ")") {
		// extract between first '(' and last ')'
		start := strings.Index(w, "(")
		end := strings.LastIndex(w, ")")
		if start == -1 || end == -1 || end <= start+1 {
			return nil
		}
		inside := strings.TrimSpace(w[start+1 : end])
		parts := strings.FieldsFunc(inside, func(r rune) bool { return r == ' ' || r == ',' })
		if len(parts) >= 2 {
			lonStr := strings.TrimSpace(parts[0])
			latStr := strings.TrimSpace(parts[1])
			lon, err1 := strconv.ParseFloat(lonStr, 64)
			lat, err2 := strconv.ParseFloat(latStr, 64)
			if err1 == nil && err2 == nil {
				return map[string]interface{}{
					"type":        "Point",
					"coordinates": []float64{lon, lat},
				}
			}
		}
	}
	// unsupported or invalid geometry -> return nil
	return nil
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
