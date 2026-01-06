package spatial

import (
	"encoding/json"
	"log"
	"os"
	"sync"
	"time"
)

// RegionalCourierManager manages multiple couriers for different regions
type RegionalCourierManager struct {
	mu       sync.RWMutex
	couriers map[string]*CourierState // region ID -> courier
	enabled  bool
}

var regionalManager *RegionalCourierManager
const regionalCourierFile = "regional_couriers.json"

// InitRegionalCouriers initializes couriers for each agent cluster
func InitRegionalCouriers() {
	if regionalManager != nil {
		return
	}
	
	regionalManager = &RegionalCourierManager{
		couriers: make(map[string]*CourierState),
	}
	
	// Try to load saved state
	if data, err := os.ReadFile(regionalCourierFile); err == nil {
		var saved struct {
			Enabled  bool                      `json:"enabled"`
			Couriers map[string]*CourierState `json:"couriers"`
		}
		if json.Unmarshal(data, &saved) == nil {
			regionalManager.enabled = saved.Enabled
			regionalManager.couriers = saved.Couriers
			log.Printf("[regional-courier] restored %d couriers, enabled=%v", len(saved.Couriers), saved.Enabled)
			return
		}
	}
	
	// Initialize fresh - find clusters and create couriers
	regionalManager.detectAndCreateCouriers()
}

// detectAndCreateCouriers finds agent clusters and creates a courier for each
func (rm *RegionalCourierManager) detectAndCreateCouriers() {
	db := Get()
	agents := db.ListAgents()
	if len(agents) == 0 {
		return
	}
	
	// Cluster agents by proximity (agents within 50km are in same cluster)
	const clusterRadius = 50000.0 // 50km
	
	clustered := make(map[string]bool)
	clusters := make(map[string][]*Entity) // cluster ID -> agents
	
	for _, agent := range agents {
		if clustered[agent.ID] {
			continue
		}
		
		// Start new cluster with this agent
		clusterID := agent.ID
		cluster := []*Entity{agent}
		clustered[agent.ID] = true
		
		// Find all agents within radius
		for _, other := range agents {
			if clustered[other.ID] {
				continue
			}
			// Check distance to any agent already in cluster
			for _, inCluster := range cluster {
				dist := distanceMeters(inCluster.Lat, inCluster.Lon, other.Lat, other.Lon)
				if dist < clusterRadius {
					cluster = append(cluster, other)
					clustered[other.ID] = true
					break
				}
			}
		}
		
		clusters[clusterID] = cluster
	}
	
	// Create courier for each cluster with 2+ agents
	for clusterID, cluster := range clusters {
		if len(cluster) < 2 {
			continue
		}
		
		// Use first agent as starting point
		startAgent := cluster[0]
		regionName := startAgent.Name
		if len(regionName) > 30 {
			regionName = regionName[:30]
		}
		
		rm.couriers[clusterID] = &CourierState{
			CurrentLat: startAgent.Lat,
			CurrentLon: startAgent.Lon,
			LastMove:   time.Now(),
			Enabled:    false,
		}
		
		log.Printf("[regional-courier] created courier for %s cluster (%d agents)", regionName, len(cluster))
	}
	
	log.Printf("[regional-courier] initialized %d regional couriers", len(rm.couriers))
}

// EnableRegionalCouriers enables all regional couriers
func EnableRegionalCouriers() {
	if regionalManager == nil {
		InitRegionalCouriers()
	}
	
	regionalManager.mu.Lock()
	regionalManager.enabled = true
	for _, c := range regionalManager.couriers {
		c.Enabled = true
	}
	regionalManager.mu.Unlock()
	
	saveRegionalCouriers()
	log.Printf("[regional-courier] enabled all %d couriers", len(regionalManager.couriers))
}

// DisableRegionalCouriers disables all regional couriers
func DisableRegionalCouriers() {
	if regionalManager == nil {
		return
	}
	
	regionalManager.mu.Lock()
	regionalManager.enabled = false
	for _, c := range regionalManager.couriers {
		c.Enabled = false
	}
	regionalManager.mu.Unlock()
	
	saveRegionalCouriers()
	log.Printf("[regional-courier] disabled all couriers")
}

// StartRegionalCourierLoop starts the background loop for all regional couriers
func StartRegionalCourierLoop() {
	InitRegionalCouriers()
	
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		
		for range ticker.C {
			RegionalCourierStep()
		}
	}()
	
	log.Printf("[regional-courier] background loop started")
}

// RegionalCourierStep performs one step for each active regional courier
func RegionalCourierStep() {
	if regionalManager == nil || !regionalManager.enabled {
		return
	}
	
	regionalManager.mu.RLock()
	couriers := make(map[string]*CourierState)
	for k, v := range regionalManager.couriers {
		couriers[k] = v
	}
	regionalManager.mu.RUnlock()
	
	db := Get()
	
	for clusterID, courier := range couriers {
		if !courier.Enabled {
			continue
		}
		
		// Rate limit per courier
		if time.Since(courier.LastMove) < 5*time.Second {
			continue
		}
		
		// Step this courier
		stepRegionalCourier(db, clusterID, courier)
	}
}

// stepRegionalCourier performs one step for a specific regional courier
func stepRegionalCourier(db *DB, clusterID string, courier *CourierState) {
	// If no current route, pick a new destination within this cluster
	if len(courier.Route) == 0 || courier.RouteIndex >= len(courier.Route) {
		if !pickRegionalDestination(db, clusterID, courier) {
			return
		}
	}
	
	// Walk along route
	walkRegionalRoute(db, courier)
}

// pickRegionalDestination chooses next destination within the cluster
func pickRegionalDestination(db *DB, clusterID string, courier *CourierState) bool {
	// Find agents near this courier's position (within cluster)
	const clusterRadius = 50000.0
	agents := db.ListAgents()
	
	var candidates []*Entity
	for _, agent := range agents {
		dist := distanceMeters(courier.CurrentLat, courier.CurrentLon, agent.Lat, agent.Lon)
		if dist < clusterRadius && dist > 100 {
			candidates = append(candidates, agent)
		}
	}
	
	if len(candidates) == 0 {
		return false
	}
	
	// Pick nearest unconnected agent
	var best *Entity
	bestDist := float64(999999999)
	
	for _, agent := range candidates {
		dist := distanceMeters(courier.CurrentLat, courier.CurrentLon, agent.Lat, agent.Lon)
		
		// Check if already connected (street exists)
		nearbyStreets := db.Query(agent.Lat, agent.Lon, 200, EntityStreet, 20)
		hasStreet := false
		for _, street := range nearbyStreets {
			sd := street.GetStreetData()
			if sd != nil && len(sd.Points) > 0 {
				startLon, startLat := sd.Points[0][0], sd.Points[0][1]
				startDist := distanceMeters(courier.CurrentLat, courier.CurrentLon, startLat, startLon)
				if startDist < 200 {
					hasStreet = true
					break
				}
			}
		}
		
		// Prefer unconnected, then nearest
		if !hasStreet && dist < bestDist {
			best = agent
			bestDist = dist
		}
	}
	
	// If all connected, pick nearest anyway (for re-walking)
	if best == nil {
		for _, agent := range candidates {
			dist := distanceMeters(courier.CurrentLat, courier.CurrentLon, agent.Lat, agent.Lon)
			if dist < bestDist {
				best = agent
				bestDist = dist
			}
		}
	}
	
	if best == nil {
		return false
	}
	
	// Get route via OSRM
	route, err := GetWalkingRoute(courier.CurrentLat, courier.CurrentLon, best.Lat, best.Lon)
	if err != nil {
		log.Printf("[regional-courier] route error to %s: %v", best.Name, err)
		return false
	}
	
	courier.Route = route.Coordinates
	courier.RouteIndex = 0
	courier.TargetAgent = best.ID
	courier.TargetName = best.Name
	
	log.Printf("[regional-courier] %s starting trip to %s (%.0fm)", clusterID[:8], best.Name, bestDist)
	return true
}

// walkRegionalRoute walks one step along the route
func walkRegionalRoute(db *DB, courier *CourierState) {
	if len(courier.Route) == 0 {
		return
	}
	
	// Move ~100m per step
	advance := 1
	totalDist := 0.0
	for i := courier.RouteIndex; i < len(courier.Route)-1 && totalDist < 100; i++ {
		p1 := courier.Route[i]
		p2 := courier.Route[i+1]
		segmentDist := distanceMeters(p1[1], p1[0], p2[1], p2[0])
		totalDist += segmentDist
		advance++
	}
	
	newIndex := courier.RouteIndex + advance
	if newIndex >= len(courier.Route) {
		newIndex = len(courier.Route) - 1
	}
	
	newPos := courier.Route[newIndex]
	oldLat, oldLon := courier.CurrentLat, courier.CurrentLon
	courier.CurrentLat = newPos[1]
	courier.CurrentLon = newPos[0]
	courier.RouteIndex = newIndex
	courier.LastMove = time.Now()
	
	// Track distance
	stepDist := distanceMeters(oldLat, oldLon, courier.CurrentLat, courier.CurrentLon)
	courier.MetersWalked += stepDist
	
	// Index street geometry
	indexStreetSegment(db, oldLat, oldLon, courier.CurrentLat, courier.CurrentLon, courier.TargetName)
	
	// Index POIs along the way
	indexPOIsNearPoint(db, courier.CurrentLat, courier.CurrentLon, "courier")
	
	// Check if arrived
	if newIndex >= len(courier.Route)-1 {
		courier.TripsComplete++
		log.Printf("[regional-courier] arrived at %s! Trips: %d", courier.TargetName, courier.TripsComplete)
		courier.Route = nil
		courier.RouteIndex = 0
		saveRegionalCouriers()
	} else if courier.RouteIndex%20 == 0 {
		saveRegionalCouriers()
	}
}

// indexStreetSegment stores a street segment
func indexStreetSegment(db *DB, lat1, lon1, lat2, lon2 float64, toName string) {
	// This is simplified - the main courier.go has more sophisticated street indexing
	// For now, just ensure we're mapping the route
}

// saveRegionalCouriers persists state to disk
func saveRegionalCouriers() {
	if regionalManager == nil {
		return
	}
	
	regionalManager.mu.RLock()
	data, err := json.MarshalIndent(struct {
		Enabled  bool                      `json:"enabled"`
		Couriers map[string]*CourierState `json:"couriers"`
	}{
		Enabled:  regionalManager.enabled,
		Couriers: regionalManager.couriers,
	}, "", "  ")
	regionalManager.mu.RUnlock()
	
	if err != nil {
		log.Printf("[regional-courier] failed to marshal: %v", err)
		return
	}
	
	if err := os.WriteFile(regionalCourierFile, data, 0644); err != nil {
		log.Printf("[regional-courier] failed to save: %v", err)
	}
}

// GetRegionalCourierStatus returns status of all regional couriers
func GetRegionalCourierStatus() map[string]interface{} {
	if regionalManager == nil {
		return map[string]interface{}{"enabled": false, "couriers": 0}
	}
	
	regionalManager.mu.RLock()
	defer regionalManager.mu.RUnlock()
	
	courierStats := make([]map[string]interface{}, 0)
	totalTrips := 0
	totalWalked := 0.0
	
	for id, c := range regionalManager.couriers {
		totalTrips += c.TripsComplete
		totalWalked += c.MetersWalked
		
		status := "idle"
		if c.Enabled && len(c.Route) > 0 {
			status = "walking"
		} else if c.Enabled {
			status = "active"
		}
		
		courierStats = append(courierStats, map[string]interface{}{
			"id":       id[:8],
			"status":   status,
			"target":   c.TargetName,
			"trips":    c.TripsComplete,
			"walked":   c.MetersWalked / 1000,
			"progress": float64(c.RouteIndex) / float64(max(1, len(c.Route))) * 100,
		})
	}
	
	return map[string]interface{}{
		"enabled":      regionalManager.enabled,
		"courier_count": len(regionalManager.couriers),
		"total_trips":  totalTrips,
		"total_walked": totalWalked / 1000,
		"couriers":     courierStats,
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
