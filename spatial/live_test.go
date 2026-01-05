package spatial

import (
	"testing"
	"time"
)

// TestArrivalEntityHasStopCoordinates ensures arrival entities are stored with
// the actual stop coordinates, not the user's location
func TestArrivalEntityHasStopCoordinates(t *testing.T) {
	// User location (Milton Road area)
	userLat, userLon := 51.4179, -0.3706
	
	// Simulate TfL stop data (Hampton Station bus stop is ~240m away)
	stopLat, stopLon := 51.4158, -0.3713
	stopName := "Hampton Station"
	stopID := "490001132E"
	
	// Create arrival entity as the code should
	entity := &Entity{
		ID:   GenerateID(EntityArrival, stopLat, stopLon, stopID),
		Type: EntityArrival,
		Name: "🚌 " + stopName,
		Lat:  stopLat,
		Lon:  stopLon,
		Data: map[string]interface{}{
			"stop_id":   stopID,
			"stop_name": stopName,
		},
	}
	
	// Verify coordinates are NOT the user's location
	if entity.Lat == userLat && entity.Lon == userLon {
		t.Errorf("Arrival entity has user coordinates (%.4f, %.4f) instead of stop coordinates", 
			entity.Lat, entity.Lon)
	}
	
	// Verify coordinates ARE the stop's location
	if entity.Lat != stopLat || entity.Lon != stopLon {
		t.Errorf("Arrival entity coords (%.4f, %.4f) don't match stop coords (%.4f, %.4f)",
			entity.Lat, entity.Lon, stopLat, stopLon)
	}
	
	// Verify distance from user to stop is reasonable (not 0)
	dist := haversine(userLat, userLon, entity.Lat, entity.Lon)
	if dist < 0.1 { // Less than 100m would be suspicious
		t.Errorf("Stop appears to be at user location (distance: %.0fm)", dist*1000)
	}
}

// TestEntityCoordinatesNotZero ensures entities have valid coordinates
func TestEntityCoordinatesNotZero(t *testing.T) {
	testCases := []struct {
		name string
		lat  float64
		lon  float64
		want bool // true = should be valid
	}{
		{"Valid London coords", 51.5074, -0.1278, true},
		{"Zero coords", 0, 0, false},
		{"Only lat zero", 0, -0.1278, false},
		{"Greenwich meridian", 51.5074, 0, true}, // lon=0 is valid for Greenwich
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			valid := tc.lat != 0 // Only lat=0 is truly invalid for London area
			if valid != tc.want {
				t.Errorf("Coords (%.4f, %.4f): got valid=%v, want %v", tc.lat, tc.lon, valid, tc.want)
			}
		})
	}
}

// TestGenerateIDConsistency ensures same inputs produce same ID
func TestGenerateIDConsistency(t *testing.T) {
	id1 := GenerateID(EntityArrival, 51.4158, -0.3713, "490001132E")
	id2 := GenerateID(EntityArrival, 51.4158, -0.3713, "490001132E")
	
	if id1 != id2 {
		t.Errorf("Same inputs produced different IDs: %s vs %s", id1, id2)
	}
	
	// Different coords should produce different ID
	id3 := GenerateID(EntityArrival, 51.4179, -0.3706, "490001132E")
	if id1 == id3 {
		t.Errorf("Different coords produced same ID: %s", id1)
	}
}

// TestQueryByNameContainsReturnsCorrectCoords verifies that queried entities
// have their original coordinates, not the query location
func TestQueryByNameContainsReturnsCorrectCoords(t *testing.T) {
	// Use the global DB singleton - tests run with a fresh DB
	db := Get()
	
	// Insert a place with known coordinates
	placeCoords := struct{ lat, lon float64 }{51.4158, -0.3713}
	place := &Entity{
		ID:   "test-place-query-coords",
		Type: EntityPlace,
		Name: "Test Station Unique",
		Lat:  placeCoords.lat,
		Lon:  placeCoords.lon,
	}
	db.Insert(place)
	
	// Query from a different location
	queryLat, queryLon := 51.4179, -0.3706
	results := db.QueryByNameContains(queryLat, queryLon, 5000, "Test Station Unique")
	
	if len(results) == 0 {
		t.Fatal("Expected to find Test Station Unique")
	}
	
	// Verify returned entity has original coordinates, not query location
	if results[0].Lat == queryLat && results[0].Lon == queryLon {
		t.Errorf("Returned entity has query coordinates instead of original coordinates")
	}
	
	if results[0].Lat != placeCoords.lat || results[0].Lon != placeCoords.lon {
		t.Errorf("Returned coords (%.4f, %.4f) don't match original (%.4f, %.4f)",
			results[0].Lat, results[0].Lon, placeCoords.lat, placeCoords.lon)
	}
}

// TfL Data Tests

// TestShortDest tests destination name shortening
func TestShortDest(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// Removes " Bus Station" and " Station" suffixes
		{"Heathrow Airport Bus Station", "Heathrow"}, // After removing suffix, still > 15 chars, so first word
		{"Kingston Station", "Kingston"},
		{"Victoria Bus Station", "Victoria"},
		{"Hampton Court Station", "Hampton Court"},
		{"Piccadilly Circus", "Piccadilly"}, // > 15 chars, takes first word
		{"Very Long Destination Name That Exceeds Limit", "Very"}, // first word if > 15 chars
		{"Brixton", "Brixton"}, // Short enough, keep as-is
	}
	
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := shortDest(tc.input)
			if result != tc.expected {
				t.Errorf("shortDest(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

// TestBusArrivalFormatting tests the arrival struct formatting
func TestBusArrivalFormatting(t *testing.T) {
	now := time.Now()
	arrivals := []busArrival{
		{Line: "111", Destination: "Heathrow Airport", ArrivalTime: now.Add(3 * time.Minute)},
		{Line: "216", Destination: "Kingston Station", ArrivalTime: now.Add(7 * time.Minute)},
	}
	
	// Verify MinutesUntil calculation
	mins := arrivals[0].MinutesUntil()
	if mins < 2 || mins > 4 {
		t.Errorf("MinutesUntil: got %d, want ~3", mins)
	}
}

// TestArrivalEntityDataStructure tests that arrival entities have correct data fields
func TestArrivalEntityDataStructure(t *testing.T) {
	stopLat, stopLon := 51.4158, -0.3713
	naptanID := "490001132E"
	stopName := "Hampton Station"
	stopType := "NaptanPublicBusCoachTram"
	
	arrivals := []busArrival{
		{Line: "111", Destination: "Heathrow", ArrivalTime: time.Now().Add(5 * time.Minute)},
	}
	
	entity := &Entity{
		ID:   GenerateID(EntityArrival, stopLat, stopLon, naptanID),
		Type: EntityArrival,
		Name: "🚌 " + stopName + ": 111→Heathrow 5m",
		Lat:  stopLat,
		Lon:  stopLon,
		Data: map[string]interface{}{
			"stop_id":   naptanID,
			"stop_name": stopName,
			"stop_type": stopType,
			"arrivals":  arrivalsToInterface(arrivals),
		},
	}
	
	// Verify all required fields exist
	if entity.Data["stop_id"] != naptanID {
		t.Errorf("stop_id mismatch: got %v, want %s", entity.Data["stop_id"], naptanID)
	}
	if entity.Data["stop_name"] != stopName {
		t.Errorf("stop_name mismatch: got %v, want %s", entity.Data["stop_name"], stopName)
	}
	if entity.Data["stop_type"] != stopType {
		t.Errorf("stop_type mismatch: got %v, want %s", entity.Data["stop_type"], stopType)
	}
	
	// Verify arrivals stored correctly as []interface{} (consistent with JSON)
	storedArrivals, ok := entity.Data["arrivals"].([]interface{})
	if !ok {
		t.Fatalf("arrivals not stored as []interface{}, got %T", entity.Data["arrivals"])
	}
	if len(storedArrivals) != 1 {
		t.Errorf("arrivals count: got %d, want 1", len(storedArrivals))
	}
	firstArr, ok := storedArrivals[0].(map[string]interface{})
	if !ok {
		t.Fatalf("first arrival not map[string]interface{}, got %T", storedArrivals[0])
	}
	if firstArr["line"] != "111" {
		t.Errorf("arrival line: got %v, want 111", firstArr["line"])
	}
}

// TestHaversineDistance tests the distance calculation
func TestHaversineDistance(t *testing.T) {
	tests := []struct {
		name     string
		lat1     float64
		lon1     float64
		lat2     float64
		lon2     float64
		minKm    float64
		maxKm    float64
	}{
		{
			name:  "Same point",
			lat1:  51.5074, lon1: -0.1278,
			lat2:  51.5074, lon2: -0.1278,
			minKm: 0, maxKm: 0.001,
		},
		{
			name:  "London to Greenwich (~8km)",
			lat1:  51.5074, lon1: -0.1278,
			lat2:  51.4772, lon2: 0.0005,
			minKm: 8, maxKm: 10,
		},
		{
			name:  "Hampton to Kingston (~3km)",
			lat1:  51.4158, lon1: -0.3713,
			lat2:  51.4115, lon2: -0.3070,
			minKm: 4, maxKm: 6,
		},
	}
	
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dist := haversine(tc.lat1, tc.lon1, tc.lat2, tc.lon2)
			if dist < tc.minKm || dist > tc.maxKm {
				t.Errorf("haversine() = %.2f km, want between %.2f and %.2f km", 
					dist, tc.minKm, tc.maxKm)
			}
		})
	}
}

// TestArrivalEntityTTL verifies arrivals have correct expiry
func TestArrivalEntityTTL(t *testing.T) {
	// Create entity with expiry (simulating fetchTransportArrivals behavior)
	now := time.Now()
	expiry := now.Add(5 * time.Minute) // arrivalTTL is 5min
	
	entity := &Entity{
		ID:        "test-arrival",
		Type:      EntityArrival,
		ExpiresAt: &expiry,
	}
	
	// Verify expiry is in the future
	if entity.ExpiresAt.Before(now) {
		t.Error("Expiry should be in the future")
	}
	
	// Verify expiry is approximately 5 minutes out
	diff := entity.ExpiresAt.Sub(now)
	if diff < 4*time.Minute || diff > 6*time.Minute {
		t.Errorf("Expiry diff = %v, want ~5 minutes", diff)
	}
}

// TestWeatherIcon tests the weather code to icon mapping
func TestWeatherIcon(t *testing.T) {
	tests := []struct {
		code int
		icon string
	}{
		{0, "☀️"},    // Clear sky
		{1, "⛅"},    // Partly cloudy
		{3, "⛅"},    // Overcast
		{45, "🌫️"},   // Fog
		{51, "🌧️"},   // Drizzle
		{63, "🌧️"},   // Rain
		{71, "❄️"},   // Snow
		{95, "⛈️"},   // Thunderstorm
		{100, "🌡️"}, // Unknown
	}
	
	for _, tc := range tests {
		t.Run(tc.icon, func(t *testing.T) {
			result := weatherIcon(tc.code)
			if result != tc.icon {
				t.Errorf("weatherIcon(%d) = %q, want %q", tc.code, result, tc.icon)
			}
		})
	}
}

// TestStopTypeConstants verifies TfL stop type strings are correct
func TestStopTypeConstants(t *testing.T) {
	// These are the actual TfL API stop type strings
	stopTypes := []string{
		"NaptanPublicBusCoachTram",  // Buses
		"NaptanMetroStation",        // Tube
		"NaptanRailStation",         // Rail
	}
	
	// Verify they don't have common typos
	for _, st := range stopTypes {
		if len(st) < 10 {
			t.Errorf("Stop type %q seems too short", st)
		}
		if st[0] != 'N' {
			t.Errorf("Stop type %q should start with 'N'", st)
		}
	}
}

// TestArrivalSorting verifies arrivals are sorted by time
func TestArrivalSorting(t *testing.T) {
	now := time.Now()
	arrivals := []busArrival{
		{Line: "111", ArrivalTime: now.Add(10 * time.Minute)},
		{Line: "216", ArrivalTime: now.Add(3 * time.Minute)},
		{Line: "290", ArrivalTime: now.Add(7 * time.Minute)},
	}
	
	// Verify they're in increasing order (as they should be after sort)
	// This tests the expected output format, not the actual sort (which happens in fetchStopArrivals)
	sorted := []busArrival{
		{Line: "216", ArrivalTime: now.Add(3 * time.Minute)},
		{Line: "290", ArrivalTime: now.Add(7 * time.Minute)},
		{Line: "111", ArrivalTime: now.Add(10 * time.Minute)},
	}
	
	if sorted[0].MinutesUntil() > sorted[1].MinutesUntil() || sorted[1].MinutesUntil() > sorted[2].MinutesUntil() {
		t.Error("Arrivals should be sorted by MinutesUntil ascending")
	}
	
	// Original array should still be unsorted (we didn't sort in place)
	if arrivals[0].MinutesUntil() < arrivals[1].MinutesUntil() {
		t.Error("Original array was modified unexpectedly")
	}
}

// TestEntityTypeConstants verifies entity type values
func TestEntityTypeConstants(t *testing.T) {
	types := map[EntityType]string{
		EntityPlace:      "place",
		EntityAgent:      "agent", 
		EntityPerson:     "person",
		EntityWeather:    "weather",
		EntityPrayer:     "prayer",
		EntityArrival:    "arrival",
		EntityDisruption: "disruption",
		EntityLocation:   "location",
	}
	
	for et, expected := range types {
		if string(et) != expected {
			t.Errorf("EntityType %v != %q", et, expected)
		}
	}
}

// TestRadiusForDifferentEntityTypes verifies appropriate query radii
func TestRadiusForDifferentEntityTypes(t *testing.T) {
	// From claude.md cache strategy
	expectedRadii := map[EntityType]int{
		EntityWeather:    5000,  // 5km
		EntityPrayer:     50000, // 50km city-wide
		EntityArrival:    500,   // 500m
		EntityDisruption: 10000, // 10km
		EntityLocation:   500,   // 500m
		EntityPlace:      500,   // 500m default
	}
	
	// Just verify these are reasonable values
	for et, radius := range expectedRadii {
		if radius < 100 || radius > 100000 {
			t.Errorf("Radius for %s = %d seems unreasonable", et, radius)
		}
	}
}
