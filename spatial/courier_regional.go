package spatial

import (
	"encoding/json"
	"fmt"
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
			Enabled  bool                     `json:"enabled"`
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
		// If this was a manual target, check for next waypoint
		if courier.ManualTarget {
			log.Printf("[regional-courier] %s: arrived at manual target %s", clusterID[:8], courier.TargetName)

			// Check for next waypoint in queue
			next := popNextWaypoint()
			if next != nil {
				log.Printf("[regional-courier] %s: next waypoint %s", clusterID[:8], next.Name)
				route, err := GetWalkingRoute(courier.CurrentLat, courier.CurrentLon, next.Lat, next.Lon)
				if err == nil {
					courier.Route = route.Coordinates
					courier.RouteIndex = 0
					courier.TargetName = next.Name
					// Keep ManualTarget = true for the next leg
					saveRegionalCouriers()
					walkRegionalRoute(db, courier)
					return
				}
				log.Printf("[regional-courier] %s: route error to %s: %v", clusterID[:8], next.Name, err)
			}

			// No more waypoints or route failed, resume auto
			courier.ManualTarget = false
			saveRegionalCouriers()
		}
		if !pickRegionalDestination(db, clusterID, courier) {
			return
		}
	}

	// Walk along route
	walkRegionalRoute(db, courier)
}

// pickRegionalDestination chooses next destination to maximize network connectivity
func pickRegionalDestination(db *DB, clusterID string, courier *CourierState) bool {
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

	// Build connectivity graph - which agents are connected via streets?
	connected := buildConnectivityGraph(db, candidates)

	// Find which component the courier is currently in
	courierComponent := findComponent(courier.CurrentLat, courier.CurrentLon, candidates, connected)

	// Strategy 1: Find agent in a DIFFERENT component (bridges isolated areas)
	// Find all different components and their nearest representatives
	componentAgents := make(map[string]*Entity) // component -> nearest agent in that component
	componentDists := make(map[string]float64)

	for _, agent := range candidates {
		agentComponent := findComponent(agent.Lat, agent.Lon, candidates, connected)
		if agentComponent != courierComponent && agentComponent != "" {
			dist := distanceMeters(courier.CurrentLat, courier.CurrentLon, agent.Lat, agent.Lon)
			if _, exists := componentDists[agentComponent]; !exists || dist < componentDists[agentComponent] {
				componentAgents[agentComponent] = agent
				componentDists[agentComponent] = dist
			}
		}
	}

	// Pick the FARTHEST component (to expand coverage faster)
	var bridgeCandidate *Entity
	bridgeDist := float64(0)

	for comp, agent := range componentAgents {
		dist := componentDists[comp]
		if dist > bridgeDist {
			bridgeCandidate = agent
			bridgeDist = dist
		}
	}

	if bridgeCandidate != nil {
		log.Printf("[regional-courier] %s: bridging to %s (different component, %.0fm)", clusterID[:8], bridgeCandidate.Name, bridgeDist)
		return getRouteToAgent(courier, bridgeCandidate)
	}

	// Strategy 2: Find least-connected agent (has fewest street connections)
	var leastConnected *Entity
	minConnections := 999
	leastDist := float64(999999999)

	for _, agent := range candidates {
		conns := countConnections(db, agent)
		dist := distanceMeters(courier.CurrentLat, courier.CurrentLon, agent.Lat, agent.Lon)

		// Prefer fewer connections, then farther away (to expand network)
		if conns < minConnections || (conns == minConnections && dist > leastDist) {
			minConnections = conns
			leastConnected = agent
			leastDist = dist
		}
	}

	if leastConnected != nil && minConnections < 3 {
		log.Printf("[regional-courier] %s: connecting to %s (only %d connections, %.0fm)", clusterID[:8], leastConnected.Name, minConnections, leastDist)
		return getRouteToAgent(courier, leastConnected)
	}

	// Strategy 3: Pick farthest unvisited agent (expand coverage)
	var farthest *Entity
	farthestDist := float64(0)

	for _, agent := range candidates {
		dist := distanceMeters(courier.CurrentLat, courier.CurrentLon, agent.Lat, agent.Lon)
		if dist > farthestDist {
			farthest = agent
			farthestDist = dist
		}
	}

	if farthest != nil {
		log.Printf("[regional-courier] %s: expanding to %s (farthest, %.0fm)", clusterID[:8], farthest.Name, farthestDist)
		return getRouteToAgent(courier, farthest)
	}

	return false
}

// buildConnectivityGraph returns a map of agent pairs that are connected via streets
func buildConnectivityGraph(db *DB, agents []*Entity) map[string]map[string]bool {
	connected := make(map[string]map[string]bool)

	for _, agent := range agents {
		connected[agent.ID] = make(map[string]bool)
	}

	// Check each agent pair for street connectivity
	for i, a1 := range agents {
		for j, a2 := range agents {
			if i >= j {
				continue
			}

			// Check if there's a street connecting them (within 500m of each)
			if areAgentsConnected(db, a1, a2) {
				connected[a1.ID][a2.ID] = true
				connected[a2.ID][a1.ID] = true
			}
		}
	}

	return connected
}

// areAgentsConnected checks if two agents have a street path between them
func areAgentsConnected(db *DB, a1, a2 *Entity) bool {
	// Simple heuristic: check if any street near a1 ends near a2
	streets := db.Query(a1.Lat, a1.Lon, 1000, EntityStreet, 50)

	for _, street := range streets {
		sd := street.GetStreetData()
		if sd == nil || len(sd.Points) < 2 {
			continue
		}

		// Check if street ends near a2
		endLon, endLat := sd.Points[len(sd.Points)-1][0], sd.Points[len(sd.Points)-1][1]
		endDist := distanceMeters(endLat, endLon, a2.Lat, a2.Lon)
		if endDist < 500 {
			return true
		}

		// Also check start (streets can be walked both ways)
		startLon, startLat := sd.Points[0][0], sd.Points[0][1]
		startDist := distanceMeters(startLat, startLon, a2.Lat, a2.Lon)
		if startDist < 500 {
			return true
		}
	}

	return false
}

// findComponent returns a component ID for the given position using union-find
func findComponent(lat, lon float64, agents []*Entity, connected map[string]map[string]bool) string {
	// Find nearest agent
	var nearest *Entity
	minDist := float64(999999999)

	for _, agent := range agents {
		dist := distanceMeters(lat, lon, agent.Lat, agent.Lon)
		if dist < minDist {
			minDist = dist
			nearest = agent
		}
	}

	if nearest == nil || minDist > 2000 {
		return ""
	}

	// BFS to find all agents in same component
	visited := make(map[string]bool)
	queue := []string{nearest.ID}
	visited[nearest.ID] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for neighbor := range connected[current] {
			if !visited[neighbor] {
				visited[neighbor] = true
				queue = append(queue, neighbor)
			}
		}
	}

	// Return smallest ID in component as component identifier
	minID := nearest.ID
	for id := range visited {
		if id < minID {
			minID = id
		}
	}

	return minID
}

// countConnections returns how many street connections an agent has
func countConnections(db *DB, agent *Entity) int {
	streets := db.Query(agent.Lat, agent.Lon, 500, EntityStreet, 50)
	return len(streets)
}

// getRouteToAgent gets walking route to an agent
func getRouteToAgent(courier *CourierState, agent *Entity) bool {
	route, err := GetWalkingRoute(courier.CurrentLat, courier.CurrentLon, agent.Lat, agent.Lon)
	if err != nil {
		log.Printf("[regional-courier] route error to %s: %v", agent.Name, err)
		return false
	}

	courier.Route = route.Coordinates
	courier.RouteIndex = 0
	courier.TargetAgent = agent.ID
	courier.TargetName = agent.Name

	log.Printf("[regional-courier] %s starting trip to %s (%.0fm)", courier.TargetAgent[:8], agent.Name, route.Distance)
	return true
}
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
		Enabled  bool                     `json:"enabled"`
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
		"enabled":       regionalManager.enabled,
		"courier_count": len(regionalManager.couriers),
		"total_trips":   totalTrips,
		"total_walked":  totalWalked / 1000,
		"couriers":      courierStats,
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// SendCourierTo forces the nearest courier to go to a specific location
func SendCourierTo(lat, lon float64, name string) error {
	if regionalManager == nil {
		return fmt.Errorf("couriers not initialized")
	}

	// Find nearest courier (read lock)
	regionalManager.mu.RLock()
	if !regionalManager.enabled {
		regionalManager.mu.RUnlock()
		return fmt.Errorf("couriers are disabled")
	}

	var nearestID string
	var nearestCourier *CourierState
	var startLat, startLon float64
	nearestDist := float64(999999999)

	for id, courier := range regionalManager.couriers {
		if !courier.Enabled {
			continue
		}
		dist := distanceMeters(courier.CurrentLat, courier.CurrentLon, lat, lon)
		if dist < nearestDist {
			nearestDist = dist
			nearestID = id
			nearestCourier = courier
			startLat = courier.CurrentLat
			startLon = courier.CurrentLon
		}
	}
	regionalManager.mu.RUnlock()

	if nearestCourier == nil {
		return fmt.Errorf("no active couriers found")
	}

	// Get route (no lock - this is slow)
	route, err := GetWalkingRoute(startLat, startLon, lat, lon)
	if err != nil {
		return fmt.Errorf("couldn't get route: %v", err)
	}

	// Update courier (write lock)
	regionalManager.mu.Lock()
	nearestCourier.Route = route.Coordinates
	nearestCourier.RouteIndex = 0
	nearestCourier.TargetAgent = ""
	nearestCourier.TargetName = name
	nearestCourier.ManualTarget = true // Don't auto-pick next destination
	regionalManager.mu.Unlock()

	log.Printf("[regional-courier] %s manually sent to %s (%.0fm)", nearestID[:8], name, route.Distance)

	// Save state (has its own lock)
	saveRegionalCouriers()

	return nil
}

// Waypoint represents a point in a multi-stop route
type Waypoint struct {
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
	Name string  `json:"name"`
}

// Queued waypoints for the courier
var courierWaypoints []Waypoint
var waypointsMu sync.RWMutex

// GetCourierWaypoints returns the current waypoint queue
func GetCourierWaypoints() []Waypoint {
	waypointsMu.RLock()
	defer waypointsMu.RUnlock()
	return courierWaypoints
}

// ClearCourierWaypoints clears the waypoint queue
func ClearCourierWaypoints() {
	waypointsMu.Lock()
	defer waypointsMu.Unlock()
	courierWaypoints = nil
}

// SetCourierWaypoints sets waypoints and starts the courier on the first leg
func SetCourierWaypoints(waypoints []Waypoint) error {
	if len(waypoints) == 0 {
		return fmt.Errorf("no waypoints provided")
	}

	waypointsMu.Lock()
	courierWaypoints = waypoints
	waypointsMu.Unlock()

	// Start courier to first waypoint
	first := waypoints[0]
	return SendCourierTo(first.Lat, first.Lon, first.Name)
}

// popNextWaypoint removes and returns the next waypoint, or nil if empty
func popNextWaypoint() *Waypoint {
	waypointsMu.Lock()
	defer waypointsMu.Unlock()

	if len(courierWaypoints) == 0 {
		return nil
	}

	// Remove the first waypoint (we just arrived there)
	if len(courierWaypoints) > 1 {
		courierWaypoints = courierWaypoints[1:]
		next := courierWaypoints[0]
		return &next
	}

	// Last waypoint reached
	courierWaypoints = nil
	return nil
}
