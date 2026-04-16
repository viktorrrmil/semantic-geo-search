package services

import (
    "math"
    "testing"
)

func almostEqual(a, b, eps float64) bool {
    return math.Abs(a-b) <= eps
}

func floatPtr(v float64) *float64 { return &v }

func TestParseCoordinates(t *testing.T) {
    tests := []struct {
        name      string
        geom      string
        wantLat   float64
        wantLng   float64
        wantError bool
    }{
        {"WKT space", "POINT(55.0 25.0)", 25.0, 55.0, false},
        {"WKT comma", "POINT(55.0,25.0)", 25.0, 55.0, false},
        {"GeoJSON", `{"type":"Point","coordinates":[55.0,25.0]}`, 25.0, 55.0, false},
        {"Empty", "", 0, 0, true},
    }

    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            lat, lng, err := ParseCoordinates(tc.geom)
            if tc.wantError {
                if err == nil {
                    t.Fatalf("expected error, got nil")
                }
                return
            }
            if err != nil {
                t.Fatalf("unexpected error: %v", err)
            }
            if !almostEqual(lat, tc.wantLat, 1e-9) || !almostEqual(lng, tc.wantLng, 1e-9) {
                t.Fatalf("got lat=%v lng=%v, want lat=%v lng=%v", lat, lng, tc.wantLat, tc.wantLng)
            }
        })
    }
}

func TestHaversine(t *testing.T) {
    t.Run("zero distance", func(t *testing.T) {
        d := Haversine(25.0, 55.0, 25.0, 55.0)
        if !almostEqual(d, 0.0, 1e-9) {
            t.Fatalf("expected 0.0, got %v", d)
        }
    })

    t.Run("equator 1 degree lon", func(t *testing.T) {
        want := 111.194926645
        d := Haversine(0, 0, 0, 1)
        if !almostEqual(d, want, 1e-4) {
            t.Fatalf("distance mismatch: got %v want %v", d, want)
        }
    })
}

func TestRerankResults_NoCenter_SemanticNormalization(t *testing.T) {
    results := []SearchResult{
        {ID: "r0", Distance: 0.2},
        {ID: "r1", Distance: 0.5},
        {ID: "r2", Distance: 0.8},
    }
    params := HybridRankingParams{Alpha: 0.7, Beta: 0.3}

    scored := RerankResults(results, params)
    if len(scored) != 3 {
        t.Fatalf("expected 3 results, got %d", len(scored))
    }

    // Expect semantic scores: [1.0, 0.5, 0.0]
    wantSem := []float64{1.0, 0.5, 0.0}
    for i := 0; i < 3; i++ {
        if !almostEqual(scored[i].SemanticScore, wantSem[i], 1e-6) {
            t.Fatalf("semantic score mismatch idx=%d got=%v want=%v", i, scored[i].SemanticScore, wantSem[i])
        }
        // no center => geo=0 and final equals semantic
        if !almostEqual(scored[i].GeoScore, 0.0, 1e-9) {
            t.Fatalf("expected geo=0, got %v", scored[i].GeoScore)
        }
        if !almostEqual(scored[i].FinalScore, scored[i].SemanticScore, 1e-9) {
            t.Fatalf("final should equal semantic when no center: final=%v sem=%v", scored[i].FinalScore, scored[i].SemanticScore)
        }
    }

    // Ensure sorted descending
    for i := 0; i < len(scored)-1; i++ {
        if scored[i].FinalScore < scored[i+1].FinalScore {
            t.Fatalf("results not sorted: idx %d score %v < idx %d score %v", i, scored[i].FinalScore, i+1, scored[i+1].FinalScore)
        }
    }
}

func TestRerankResults_WithCenter_GeoScoringAndCategoryBonus(t *testing.T) {
    // Two results: A (distance 0.5 at center), B (distance 0.2 slightly east)
    results := []SearchResult{
        {ID: "A", Distance: 0.5, Geom: "POINT(55.0 25.0)"},
        {ID: "B", Distance: 0.2, Geom: "POINT(55.1 25.0)"},
    }
    centerLat := 25.0
    centerLng := 55.0
    params := HybridRankingParams{Alpha: 0.7, Beta: 0.3, DecayRadiusKm: 5.0, CenterLat: &centerLat, CenterLng: &centerLng}

    scored := RerankResults(results, params)
    if len(scored) != 2 {
        t.Fatalf("expected 2 results, got %d", len(scored))
    }

    // After normalization: for distances [0.5,0.2] with min=0.2 max=0.5 => sem A=0, B=1
    // Geo: A at center => 1.0, B at ~10km => exp(-~10/5) ~ 0.1336
    var scoreA, scoreB float64
    if scored[0].ID == "B" {
        scoreB = scored[0].FinalScore
        scoreA = scored[1].FinalScore
    } else {
        scoreA = scored[0].FinalScore
        scoreB = scored[1].FinalScore
    }

    if !(scoreB > scoreA) {
        t.Fatalf("expected B to rank above A: B=%v A=%v", scoreB, scoreA)
    }

    // Check approximate values
    // Compute expected approximations
    expectedGeoA := 1.0
    // compute distance between center and B
    distB := Haversine(centerLat, centerLng, 25.0, 55.1)
    expectedGeoB := math.Exp(-distB / params.DecayRadiusKm)

    // Semantic scores
    wantSemA := 0.0
    wantSemB := 1.0

    // Find scored entries by ID to check breakdowns
    var gotA, gotB *ScoredResult
    for i := range scored {
        if scored[i].ID == "A" {
            gotA = &scored[i]
        }
        if scored[i].ID == "B" {
            gotB = &scored[i]
        }
    }
    if gotA == nil || gotB == nil {
        t.Fatalf("expected both A and B present in results")
    }

    if !almostEqual(gotA.SemanticScore, wantSemA, 1e-6) {
        t.Fatalf("A semantic mismatch: got=%v want=%v", gotA.SemanticScore, wantSemA)
    }
    if !almostEqual(gotB.SemanticScore, wantSemB, 1e-6) {
        t.Fatalf("B semantic mismatch: got=%v want=%v", gotB.SemanticScore, wantSemB)
    }

    if !almostEqual(gotA.GeoScore, expectedGeoA, 1e-6) {
        t.Fatalf("A geo mismatch: got=%v want=%v", gotA.GeoScore, expectedGeoA)
    }
    if !almostEqual(gotB.GeoScore, expectedGeoB, 1e-4) {
        t.Fatalf("B geo mismatch: got=%v want~=%v", gotB.GeoScore, expectedGeoB)
    }

    // Final manual expected
    expectedFinalA := params.Alpha*wantSemA + params.Beta*expectedGeoA
    expectedFinalB := params.Alpha*wantSemB + params.Beta*expectedGeoB
    if !almostEqual(gotA.FinalScore, expectedFinalA, 1e-5) {
        t.Fatalf("A final mismatch got=%v want=%v", gotA.FinalScore, expectedFinalA)
    }
    if !almostEqual(gotB.FinalScore, expectedFinalB, 1e-5) {
        t.Fatalf("B final mismatch got=%v want=%v", gotB.FinalScore, expectedFinalB)
    }

    // Category bonus and clamping
    // Create a scenario where semantic score is 1.0, geo=0 and alpha<1 to get final < 1.0
    results2 := []SearchResult{{ID: "X", Distance: 0.5, Geom: ""}}
    // identical distances => semantic=1.0 per implementation
    p2 := HybridRankingParams{Alpha: 0.95, Beta: 0.05, DecayRadiusKm: 5.0, CenterLat: floatPtr(25.0), CenterLng: floatPtr(55.0), Category: "park"}
    // matching category and Geom empty -> geo=0, semantic=1 -> final before bonus = 0.95
    results2[0].Category = "Park"
    scored2 := RerankResults(results2, p2)
    if len(scored2) != 1 {
        t.Fatalf("expected 1 result, got %d", len(scored2))
    }
    if !(scored2[0].FinalScore <= 1.0 && scored2[0].FinalScore >= 0.999999) {
        t.Fatalf("expected final score clamped to 1.0, got %v", scored2[0].FinalScore)
    }
}

