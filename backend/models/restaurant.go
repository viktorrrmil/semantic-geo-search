package models

import "encoding/json"

// Restaurant represents a restaurant record from the Overture dataset
type Restaurant struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Confidence float64         `json:"confidence"`
	Socials    json.RawMessage `json:"socials"`
	Geometry   string          `json:"geometry"`
}

// SearchRequest represents the request body for the search endpoint
type SearchRequest struct {
	Category string  `json:"category" binding:"required"`
	BboxMinX float64 `json:"bbox_min_x" binding:"required"`
	BboxMaxX float64 `json:"bbox_max_x" binding:"required"`
	BboxMinY float64 `json:"bbox_min_y" binding:"required"`
	BboxMaxY float64 `json:"bbox_max_y" binding:"required"`
	Limit    int     `json:"limit"`
}

// SearchResponse represents the response from the search endpoint
type SearchResponse struct {
	Restaurants []Restaurant `json:"restaurants"`
	Total       int          `json:"total"`
	Message     string       `json:"message,omitempty"`
}

// PreprocessRequest represents the request to preprocess a chunk
type PreprocessRequest struct {
	Category string  `json:"category,omitempty"` // Optional: when empty, preprocess all places in bbox
	BboxMinX float64 `json:"bbox_min_x" binding:"required"`
	BboxMaxX float64 `json:"bbox_max_x" binding:"required"`
	BboxMinY float64 `json:"bbox_min_y" binding:"required"`
	BboxMaxY float64 `json:"bbox_max_y" binding:"required"`
	ChunkID  string  `json:"chunk_id,omitempty"` // Optional, will be generated if not provided
}

// PreprocessResponse represents the response from preprocessing
type PreprocessResponse struct {
	ChunkID     string  `json:"chunk_id"`
	Category    string  `json:"category"`
	BboxMinX    float64 `json:"bbox_min_x"`
	BboxMaxX    float64 `json:"bbox_max_x"`
	BboxMinY    float64 `json:"bbox_min_y"`
	BboxMaxY    float64 `json:"bbox_max_y"`
	RecordCount int     `json:"record_count"`
	Message     string  `json:"message,omitempty"`
}

// ChunkInfo represents information about a precomputed chunk
type ChunkInfo struct {
	ChunkID     string  `json:"chunk_id"`
	Category    string  `json:"category"`
	BboxMinX    float64 `json:"bbox_min_x"`
	BboxMaxX    float64 `json:"bbox_max_x"`
	BboxMinY    float64 `json:"bbox_min_y"`
	BboxMaxY    float64 `json:"bbox_max_y"`
	RecordCount int     `json:"record_count"`
	CreatedAt   string  `json:"created_at"`
}

// ChunksListResponse represents the response listing all available chunks
type ChunksListResponse struct {
	Chunks  []ChunkInfo `json:"chunks"`
	Total   int         `json:"total"`
	Message string      `json:"message,omitempty"`
}
