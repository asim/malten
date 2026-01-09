package spatial

import (
	"encoding/json"
	"log"
	"math"
	"os"
	"sort"
	"time"
)

// CourierState tracks the courier agent's inter-area journey
type CourierState struct {
	CurrentLat    float64     `json:"current_lat"`
	CurrentLon    float64     `json:"current_lon"`
	TargetAgent   string      `json:"target_agent"` // ID of agent we're traveling to
	TargetName    string      `json:"target_name"`  // Name for logging
	RouteIndex    int         `json:"route_index"`  // Current position in route
	Route         [][]float64 `json:"route"`        // Current route coordinates
	LastMove      time.Time   `json:"last_move"`
	TripsComplete int         `json:"trips_complete"`
	MetersWalked  float64     `json:"meters_walked"`
	Enabled       bool        `json:"enabled"`
	ManualTarget  bool        `json:"manual_target"` // If true, don't auto-pick next destination
}

var courierState *CourierState

const courierStateFile = "courier_state.json"

// InitCourier initializes the courier agent
func InitCourier() *CourierState {
	if courierState != nil {
		return courierState
	}

	// Try to load from file first
	if loaded := loadCourierState(); loaded != nil {
		courierState = loaded
		log.Printf("[courier] restored state: enabled=%v, trips=%d, walked=%.1fkm",
			courierState.Enabled, courierState.TripsComplete, courierState.MetersWalked/1000)
		return courierState
	}

	// Start from Hampton (or first agent if not found)
	db := Get()
	agents := db.ListAgents()
	if len(agents) == 0 {
		log.Printf("[courier] no agents found, cannot initialize")
		return nil
	}

	// Try to find Hampton
	var startAgent *Entity
	for _, a := range agents {
		if a.Name == "Hampton" || a.Name == "Hampton, Greater London" {
			startAgent = a
			break
		}
	}
	if startAgent == nil {
		startAgent = agents[0]
	}

	courierState = &CourierState{
		CurrentLat: startAgent.Lat,
		CurrentLon: startAgent.Lon,
		LastMove:   time.Now(),
		Enabled:    false, // Disabled by default
	}

	log.Printf("[courier] initialized at %s (%.4f, %.4f)", startAgent.Name, startAgent.Lat, startAgent.Lon)
	return courierState
}

// GetCourierState returns the current courier state
func GetCourierState() *CourierState {
	return courierState
}

// EnableCourier starts the courier
func EnableCourier() {
	if courierState == nil {
		InitCourier()
	}
	if courierState != nil {
		courierState.Enabled = true
		saveCourierState()
		log.Printf("[courier] enabled")
	}
}

// DisableCourier stops the courier
func DisableCourier() {
	if courierState != nil {
		courierState.Enabled = false
		saveCourierState()
		log.Printf("[courier] disabled")
	}
}

// CourierStep performs one step of the courier's journey
// Returns true if a step was taken
func CourierStep() bool {
	if courierState == nil || !courierState.Enabled {
		return false
	}

	// Rate limit: one step per 5 seconds
	if time.Since(courierState.LastMove) < 5*time.Second {
		return false
	}

	db := Get()

	// If no current route, pick a new destination
	if len(courierState.Route) == 0 || courierState.RouteIndex >= len(courierState.Route) {
		if !pickCourierDestination(db) {
			return false
		}
	}

	// Walk along route (move ~100m per step)
	return walkCourierRoute(db)
}

// pickCourierDestination chooses the next agent to visit
func pickCourierDestination(db *DB) bool {
	agents := db.ListAgents()
	if len(agents) < 2 {
		log.Printf("[courier] need at least 2 agents to courier between")
		return false
	}

	// Find agents we haven't connected to yet (no street exists)
	// Sort by distance from current position
	type agentDist struct {
		agent      *Entity
		dist       float64
		has_street bool
	}
	var candidates []agentDist

	for _, agent := range agents {
		// Skip if we're already at this agent
		dist := DistanceMeters(courierState.CurrentLat, courierState.CurrentLon, agent.Lat, agent.Lon)
		if dist < 100 {
			continue
		}

		// Check if there's already a street connecting these points
		// Look for streets near both our current position and the target
		nearbyStreets := db.Query(agent.Lat, agent.Lon, 200, EntityStreet, 20)
		hasStreet := false
		for _, street := range nearbyStreets {
			sd := street.GetStreetData()
			if sd == nil {
				continue
			}
			// Check if this street starts near our current position
			if len(sd.Points) > 0 {
				startLon, startLat := sd.Points[0][0], sd.Points[0][1]
				startDist := DistanceMeters(courierState.CurrentLat, courierState.CurrentLon, startLat, startLon)
				if startDist < 200 {
					hasStreet = true
					break
				}
			}
		}

		candidates = append(candidates, agentDist{agent, dist, hasStreet})
	}

	if len(candidates) == 0 {
		log.Printf("[courier] no candidates found")
		return false
	}

	// Prefer unconnected agents, then by distance
	sort.Slice(candidates, func(i, j int) bool {
		// Unconnected first
		if !candidates[i].has_street && candidates[j].has_street {
			return true
		}
		if candidates[i].has_street && !candidates[j].has_street {
			return false
		}
		// Then by distance (prefer closer)
		return candidates[i].dist < candidates[j].dist
	})

	// Pick the best candidate
	target := candidates[0].agent

	// Get walking route
	route, err := GetWalkingRoute(courierState.CurrentLat, courierState.CurrentLon, target.Lat, target.Lon)
	if err != nil {
		log.Printf("[courier] route to %s failed: %v", target.Name, err)
		return false
	}

	if len(route.Coordinates) < 2 {
		log.Printf("[courier] route to %s too short", target.Name)
		return false
	}

	courierState.TargetAgent = target.ID
	courierState.TargetName = target.Name
	courierState.Route = route.Coordinates
	courierState.RouteIndex = 0

	log.Printf("[courier] starting trip to %s (%.0fm, %d points)",
		target.Name, route.Distance, len(route.Coordinates))

	// Index the full street geometry now
	street := &Entity{
		ID:   GenerateID(EntityStreet, courierState.CurrentLat, courierState.CurrentLon, target.Name),
		Type: EntityStreet,
		Name: "Courier route to " + target.Name,
		Lat:  courierState.CurrentLat,
		Lon:  courierState.CurrentLon,
		Data: &StreetData{
			Points: route.Coordinates,
			Length: route.Distance,
			ToName: target.Name,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	db.Insert(street)

	return true
}

// walkCourierRoute moves the courier along the current route
func walkCourierRoute(db *DB) bool {
	if len(courierState.Route) == 0 {
		return false
	}

	// Calculate how many route points to advance (~100m worth)
	advance := 1
	totalDist := 0.0
	for i := courierState.RouteIndex; i < len(courierState.Route)-1 && totalDist < 100; i++ {
		p1 := courierState.Route[i]
		p2 := courierState.Route[i+1]
		segmentDist := DistanceMeters(p1[1], p1[0], p2[1], p2[0])
		totalDist += segmentDist
		advance++
	}

	// Move to new position
	newIndex := courierState.RouteIndex + advance
	if newIndex >= len(courierState.Route) {
		newIndex = len(courierState.Route) - 1
	}

	newPos := courierState.Route[newIndex]
	oldLat, oldLon := courierState.CurrentLat, courierState.CurrentLon
	courierState.CurrentLat = newPos[1] // Route is [lon, lat]
	courierState.CurrentLon = newPos[0]
	courierState.RouteIndex = newIndex
	courierState.LastMove = time.Now()

	// Track distance
	stepDist := DistanceMeters(oldLat, oldLon, courierState.CurrentLat, courierState.CurrentLon)
	courierState.MetersWalked += stepDist

	// Index POIs along the way
	indexPOIsNearPoint(db, courierState.CurrentLat, courierState.CurrentLon, "courier")

	// Check if arrived
	if newIndex >= len(courierState.Route)-1 {
		courierState.TripsComplete++
		log.Printf("[courier] arrived at %s! Trips: %d, Total walked: %.1fkm",
			courierState.TargetName, courierState.TripsComplete, courierState.MetersWalked/1000)

		// Clear route so we pick a new destination
		courierState.Route = nil
		courierState.RouteIndex = 0
		saveCourierState()
	} else if courierState.RouteIndex%20 == 0 {
		// Save periodically during long routes
		saveCourierState()
	}

	return true
}

// indexPOIsNearPoint queues OSM POI indexing for a point (non-blocking)
func indexPOIsNearPoint(db *DB, lat, lon float64, agentID string) {
	// Check if we've already indexed this area recently
	existing := db.Query(lat, lon, 50, EntityPlace, 1)
	if len(existing) > 0 {
		return // Already indexed
	}

	// Do OSM query in background to avoid blocking courier walk
	go func() {
		pois := queryOSMPOIsNearby(lat, lon, 100, agentID)
		for _, poi := range pois {
			db.Insert(poi)
		}
		if len(pois) > 0 {
			log.Printf("[courier] indexed %d POIs near (%.4f, %.4f)", len(pois), lat, lon)
		}
	}()
}

// GetCourierStats returns courier statistics
func GetCourierStats() map[string]interface{} {
	if courierState == nil {
		return map[string]interface{}{
			"enabled":     false,
			"initialized": false,
		}
	}

	stats := map[string]interface{}{
		"enabled":        courierState.Enabled,
		"initialized":    true,
		"current_lat":    courierState.CurrentLat,
		"current_lon":    courierState.CurrentLon,
		"trips_complete": courierState.TripsComplete,
		"meters_walked":  courierState.MetersWalked,
		"km_walked":      math.Round(courierState.MetersWalked/100) / 10,
	}

	if courierState.TargetName != "" {
		stats["heading_to"] = courierState.TargetName
		if len(courierState.Route) > 0 {
			progress := float64(courierState.RouteIndex) / float64(len(courierState.Route)) * 100
			stats["progress"] = math.Round(progress)
		}
	}

	return stats
}

// loadCourierState loads courier state from file
func loadCourierState() *CourierState {
	data, err := os.ReadFile(courierStateFile)
	if err != nil {
		return nil
	}
	var state CourierState
	if err := json.Unmarshal(data, &state); err != nil {
		log.Printf("[courier] failed to parse state file: %v", err)
		return nil
	}
	return &state
}

// saveCourierState saves courier state to file
func saveCourierState() {
	if courierState == nil {
		return
	}
	data, err := json.MarshalIndent(courierState, "", "  ")
	if err != nil {
		log.Printf("[courier] failed to marshal state: %v", err)
		return
	}
	if err := os.WriteFile(courierStateFile, data, 0644); err != nil {
		log.Printf("[courier] failed to save state: %v", err)
	}
}

// StartCourierLoop starts the background courier loop
func StartCourierLoop() {
	InitCourier()

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			CourierStep()
		}
	}()

	log.Printf("[courier] background loop started")
}
