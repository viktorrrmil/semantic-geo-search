package services

import (
  "encoding/json"
  "errors"
  "math"
  "sort"
  "strconv"
  "strings"
)

const (
  defaultHybridAlpha         = 0.7
  defaultHybridBeta          = 0.3
  defaultHybridDecayRadiusKm = 5.0
  hybridCategoryBonus        = 0.1
)

// HybridRankingParams holds tunable parameters for reranking.
type HybridRankingParams struct {
  Alpha         float64  // weight for semantic score, default 0.7
  Beta          float64  // weight for geo score, default 0.3
  DecayRadiusKm float64  // decay radius for geo score, default 5.0
  CenterLat     *float64 // optional, nil means no geo anchor
  CenterLng     *float64 // optional, nil means no geo anchor
  Category      string   // optional category boost
}

// SearchResult represents a single search hit returned by Lynx.
type SearchResult struct {
  ID        string          `json:"id"`
  EmbedText string          `json:"embed_text"`
  Geom      any             `json:"geom"`
  Category  string          `json:"category,omitempty"`
  Country   string          `json:"country,omitempty"`
  Distance  float64         `json:"distance,omitempty"`
  Raw       json.RawMessage `json:"raw,omitempty"`
  Extra     map[string]any   `json:"-"`
}

// ScoredResult extends SearchResult with the hybrid score breakdown.
type ScoredResult struct {
  SearchResult
  SemanticScore float64 `json:"semantic_score"`
  GeoScore      float64 `json:"geo_score"`
  FinalScore    float64 `json:"final_score"`
}

// RerankResults normalizes semantic distances, computes geo scores and final scores,
// sorts by final score descending, and returns scored results.
func RerankResults(results []SearchResult, params HybridRankingParams) []ScoredResult {
  if len(results) == 0 {
    return []ScoredResult{}
  }

  alpha := params.Alpha
  if !isFinite(alpha) {
    alpha = defaultHybridAlpha
  }
  beta := params.Beta
  if !isFinite(beta) {
    beta = defaultHybridBeta
  }
  decayRadiusKm := params.DecayRadiusKm
  if !isFinite(decayRadiusKm) || decayRadiusKm <= 0 {
    decayRadiusKm = defaultHybridDecayRadiusKm
  }
  hasCenter := params.CenterLat != nil && params.CenterLng != nil
  if !hasCenter {
    alpha = 1.0
    beta = 0.0
  }

  minDistance, maxDistance := semanticDistanceRange(results)
  semanticScores := make([]float64, len(results))
  geoScores := make([]float64, len(results))
  finalScores := make([]float64, len(results))

  for i, result := range results {
    semanticScores[i] = normalizeSemanticScore(result.Distance, minDistance, maxDistance)

    if hasCenter {
        geoScores[i] = computeGeoScore(result.Geom, *params.CenterLat, *params.CenterLng, decayRadiusKm)
    } else {
      geoScores[i] = 0
    }

    final := semanticScores[i]
    if hasCenter {
      final = alpha*semanticScores[i] + beta*geoScores[i]
    }

    if matchesCategory(params.Category, result.Category) {
      final += hybridCategoryBonus
    }
    if final > 1 {
      final = 1
    } else if final < 0 {
      final = 0
    }
    finalScores[i] = final
  }

  scored := make([]ScoredResult, 0, len(results))
  for i, result := range results {
    scored = append(scored, ScoredResult{
      SearchResult:  result,
      SemanticScore: semanticScores[i],
      GeoScore:      geoScores[i],
      FinalScore:    finalScores[i],
    })
  }

  sort.SliceStable(scored, func(i, j int) bool {
    return scored[i].FinalScore > scored[j].FinalScore
  })

  return scored
}

// Haversine returns the distance in kilometers between two geographic points.
func Haversine(lat1, lng1, lat2, lng2 float64) float64 {
  const earthRadiusKm = 6371.0
  toRadians := func(v float64) float64 { return v * math.Pi / 180.0 }
  dLat := toRadians(lat2 - lat1)
  dLng := toRadians(lng2 - lng1)
  lat1Rad := toRadians(lat1)
  lat2Rad := toRadians(lat2)
  a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1Rad)*math.Cos(lat2Rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
  return 2 * earthRadiusKm * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// ParseCoordinates parses a WKT POINT or GeoJSON Point and returns latitude/longitude.
func ParseCoordinates(geom string) (lat, lng float64, err error) {
  s := strings.TrimSpace(geom)
  if s == "" {
    return 0, 0, errors.New("empty geometry")
  }

  normalized := strings.TrimSpace(stripSRIDPrefix(s))
  upper := strings.ToUpper(normalized)

  if strings.HasPrefix(upper, "POINT") {
    return parseWKTPoint(normalized)
  }

  var obj map[string]any
  if err := json.Unmarshal([]byte(normalized), &obj); err == nil {
    if lat, lng, ok := parseGeoJSONPoint(obj); ok {
      return lat, lng, nil
    }
  }

  return 0, 0, errors.New("unsupported geometry format")
}

func stripSRIDPrefix(value string) string {
  if idx := strings.Index(strings.ToUpper(value), "POINT"); idx > 0 {
    prefix := strings.TrimSpace(value[:idx])
    if strings.HasPrefix(strings.ToUpper(prefix), "SRID=") {
      return strings.TrimSpace(value[idx:])
    }
  }
  return value
}

func parseWKTPoint(value string) (lat, lng float64, err error) {
  open := strings.Index(value, "(")
  close := strings.LastIndex(value, ")")
  if open < 0 || close <= open {
    return 0, 0, errors.New("invalid WKT POINT")
  }

  inner := strings.TrimSpace(value[open+1 : close])
  inner = strings.ReplaceAll(inner, ",", " ")
  fields := strings.Fields(inner)
  if len(fields) < 2 {
    return 0, 0, errors.New("invalid WKT POINT")
  }

  x, errX := strconv.ParseFloat(fields[0], 64)
  y, errY := strconv.ParseFloat(fields[1], 64)
  if errX != nil || errY != nil {
    return 0, 0, errors.New("invalid WKT POINT")
  }
  return y, x, nil
}

func parseGeoJSONPoint(obj map[string]any) (lat, lng float64, ok bool) {
  if obj == nil {
    return 0, 0, false
  }

  if geometry, hasGeometry := obj["geometry"].(map[string]any); hasGeometry {
    if lat, lng, ok := parseGeoJSONPoint(geometry); ok {
      return lat, lng, true
    }
  }

  typeValue, _ := obj["type"].(string)
  if !strings.EqualFold(typeValue, "Point") {
    return 0, 0, false
  }

  rawCoords, ok := obj["coordinates"]
  if !ok {
    return 0, 0, false
  }

  coords := make([]any, 0, 2)
  switch v := rawCoords.(type) {
  case []any:
    coords = v
  case []float64:
    for _, c := range v {
      coords = append(coords, c)
    }
  default:
    return 0, 0, false
  }

  if len(coords) < 2 {
    return 0, 0, false
  }

  lngVal, okLng := toFloat(coords[0])
  latVal, okLat := toFloat(coords[1])
  if !okLng || !okLat {
    return 0, 0, false
  }
  return latVal, lngVal, true
}

func semanticDistanceRange(results []SearchResult) (float64, float64) {
  minDistance := math.Inf(1)
  maxDistance := math.Inf(-1)
  for _, result := range results {
    if result.Distance < minDistance {
      minDistance = result.Distance
    }
    if result.Distance > maxDistance {
      maxDistance = result.Distance
    }
  }
  return minDistance, maxDistance
}

func normalizeSemanticScore(distance, minDistance, maxDistance float64) float64 {
  if !isFinite(distance) {
    return 1
  }
  if !isFinite(minDistance) || !isFinite(maxDistance) || maxDistance == minDistance {
    return 1
  }
  score := 1 - ((distance - minDistance) / (maxDistance - minDistance))
  if score < 0 {
    return 0
  }
  if score > 1 {
    return 1
  }
  return score
}

func computeGeoScore(geom any, centerLat, centerLng, decayRadiusKm float64) float64 {
  lat, lng, err := ParseCoordinates(geometryToString(geom))
  if err != nil {
    return 0
  }
  distanceKm := Haversine(centerLat, centerLng, lat, lng)
  score := math.Exp(-distanceKm / decayRadiusKm)
  if score < 0 {
    return 0
  }
  if score > 1 {
    return 1
  }
  return score
}

func matchesCategory(expected, actual string) bool {
  if strings.TrimSpace(expected) == "" || strings.TrimSpace(actual) == "" {
    return false
  }
  return strings.EqualFold(strings.TrimSpace(expected), strings.TrimSpace(actual))
}

func toFloat(value any) (float64, bool) {
  switch v := value.(type) {
  case float64:
    return v, true
  case float32:
    return float64(v), true
  case int:
    return float64(v), true
  case int32:
    return float64(v), true
  case int64:
    return float64(v), true
  case uint:
    return float64(v), true
  case uint32:
    return float64(v), true
  case uint64:
    return float64(v), true
  case string:
    if trimmed := strings.TrimSpace(v); trimmed != "" {
      if parsed, err := strconv.ParseFloat(trimmed, 64); err == nil {
        return parsed, true
      }
    }
  }
  return 0, false
}

func isFinite(value float64) bool {
  return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func geometryToString(value any) string {
  switch v := value.(type) {
  case string:
    return strings.TrimSpace(v)
  case json.RawMessage:
    return strings.TrimSpace(string(v))
  default:
    if value == nil {
      return ""
    }
    b, err := json.Marshal(value)
    if err != nil {
      return ""
    }
    return strings.TrimSpace(string(b))
  }
}

