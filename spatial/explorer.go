package spatial

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/url"
	"time"
)

// ExplorerState tracks an agent's exploration progress
type ExplorerState struct {
	CurrentLat  float64   `json:"current_lat"`
	CurrentLon  float64   `json:"current_lon"`
	HomeLat     float64   `json:"home_lat"`
	HomeLon     float64   `json:"home_lon"`
	LastMove    time.Time `json:"last_move"`
	StepsToday  int       `json:"steps_today"`
	TotalSteps  int       `json:"total_steps"`
	Exploring   bool      `json:"exploring"`
}

var explorerStates = make(map[string]*ExplorerState)

// GetExplorerState returns the exploration state for an agent
func GetExplorerState(agentID string) *ExplorerState {
	if state, ok := explorerStates[agentID]; ok {
		return state
	}
	return nil
}

// InitExplorer initializes an agent as an explorer, loading persisted state if available
func InitExplorer(agent *Entity) *ExplorerState {
	state := &ExplorerState{
		CurrentLat: agent.Lat,
		CurrentLon: agent.Lon,
		HomeLat:    agent.Lat,
		HomeLon:    agent.Lon,
		LastMove:   time.Now(),
		Exploring:  true,
	}
	
	// Load persisted state from agent entity if available
	if ad := agent.GetAgentData(); ad != nil {
		if ad.HomeLat != 0 && ad.HomeLon != 0 {
			state.HomeLat = ad.HomeLat
			state.HomeLon = ad.HomeLon
		}
		state.TotalSteps = ad.TotalSteps
		state.StepsToday = ad.StepsToday
	}
	
	explorerStates[agent.ID] = state
	return state
}

// SaveExplorerState persists exploration state to the agent entity
func SaveExplorerState(agent *Entity, state *ExplorerState) {
	if ad := agent.GetAgentData(); ad != nil {
		ad.HomeLat = state.HomeLat
		ad.HomeLon = state.HomeLon
		ad.TotalSteps = state.TotalSteps
		ad.StepsToday = state.StepsToday
		agent.UpdatedAt = time.Now()
		Get().Insert(agent)
	}
}

// RouteGeometry contains route coordinates
type RouteGeometry struct {
	Coordinates [][]float64 // [lon, lat] pairs
	Distance    float64     // meters
	Duration    float64     // seconds
}

// GetWalkingRoute returns the geometry of a walking route
func GetWalkingRoute(fromLat, fromLon, toLat, toLon float64) (*RouteGeometry, error) {
	url := fmt.Sprintf("%s/route/v1/foot/%f,%f;%f,%f?overview=full&geometries=geojson",
		osrmBaseURL, fromLon, fromLat, toLon, toLat)
	
	resp, err := OSRMGet(url)
	if err != nil {
		return nil, fmt.Errorf("routing failed: %v", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("routing API returned %d", resp.StatusCode)
	}
	
	var data struct {
		Code   string `json:"code"`
		Routes []struct {
			Distance float64 `json:"distance"`
			Duration float64 `json:"duration"`
			Geometry struct {
				Coordinates [][]float64 `json:"coordinates"`
			} `json:"geometry"`
		} `json:"routes"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode failed: %v", err)
	}
	
	if data.Code != "Ok" || len(data.Routes) == 0 {
		return nil, fmt.Errorf("no route found")
	}
	
	route := data.Routes[0]
	return &RouteGeometry{
		Coordinates: route.Geometry.Coordinates,
		Distance:    route.Distance,
		Duration:    route.Duration,
	}, nil
}

// ExploreStep performs one exploration step for an agent
// Returns true if exploration happened, false if skipped
func ExploreStep(agent *Entity) bool {
	state := GetExplorerState(agent.ID)
	if state == nil {
		state = InitExplorer(agent)
		log.Printf("[explorer] %s: initialized explorer state", agent.Name)
	}
	
	if !state.Exploring {
		log.Printf("[explorer] %s: not exploring (state.Exploring=false)", agent.Name)
		return false
	}
	
	// Rate limit: max one move per 10 seconds
	if time.Since(state.LastMove) < 10*time.Second {
		// Don't log this - too noisy
		return false
	}
	log.Printf("[explorer] %s: starting explore step", agent.Name)
	
	db := Get()
	
	// Strategy: find an unexplored direction or nearby POI
	destination := pickExplorationTarget(db, state, agent.ID)
	if destination == nil {
		log.Printf("[explorer] %s: no exploration target found", agent.Name)
		return false
	}
	
	// Get walking route to destination
	route, err := GetWalkingRoute(state.CurrentLat, state.CurrentLon, destination.Lat, destination.Lon)
	if err != nil {
		log.Printf("[explorer] %s: route error: %v", agent.Name, err)
		return false
	}
	
	if len(route.Coordinates) < 2 {
		return false
	}
	
	// Index the street geometry
	street := &Entity{
		ID:   GenerateID(EntityStreet, state.CurrentLat, state.CurrentLon, destination.Name),
		Type: EntityStreet,
		Name: fmt.Sprintf("Route to %s", destination.Name),
		Lat:  state.CurrentLat,
		Lon:  state.CurrentLon,
		Data: &StreetData{
			Points: route.Coordinates,
			Length: route.Distance,
			ToName: destination.Name,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	db.Insert(street)
	
	// Index POIs along the route (every 100m or so)
	indexPOIsAlongRoute(db, agent.ID, route.Coordinates)
	
	// Move agent to destination
	state.CurrentLat = destination.Lat
	state.CurrentLon = destination.Lon
	state.LastMove = time.Now()
	state.StepsToday++
	state.TotalSteps++
	
	// Update agent's position in the entity
	agent.Lat = state.CurrentLat
	agent.Lon = state.CurrentLon
	agent.UpdatedAt = time.Now()
	
	// Save exploration state to agent entity (persists across restarts)
	SaveExplorerState(agent, state)
	
	log.Printf("[explorer] %s: moved to %s (%.4f, %.4f) - %d steps total, %.0fm", 
		agent.Name, destination.Name, state.CurrentLat, state.CurrentLon, state.TotalSteps, route.Distance)
	
	return true
}

type explorationTarget struct {
	Lat  float64
	Lon  float64
	Name string
}

// pickExplorationTarget chooses where to explore next
func pickExplorationTarget(db *DB, state *ExplorerState, agentID string) *explorationTarget {
	// 30% chance to just explore randomly (keeps things interesting)
	if rand.Float64() < 0.3 {
		return randomExplorationTarget(state)
	}
	
	// Strategy 1: Find a nearby POI we haven't visited via a street
	// Look for places within 500m that we don't have a street to
	places := db.Query(state.CurrentLat, state.CurrentLon, 500, EntityPlace, 50)
	
	// Shuffle places so we don't always pick the same one
	rand.Shuffle(len(places), func(i, j int) {
		places[i], places[j] = places[j], places[i]
	})
	
	for _, place := range places {
		// Skip if too close (we're already there)
		dist := distanceMeters(state.CurrentLat, state.CurrentLon, place.Lat, place.Lon)
		if dist < 50 {
			continue
		}
		
		// Check if we already have a street ending near this place (within 50m)
		streets := db.Query(place.Lat, place.Lon, 50, EntityStreet, 5)
		if len(streets) > 0 {
			continue // Already explored
		}
		
		return &explorationTarget{Lat: place.Lat, Lon: place.Lon, Name: place.Name}
	}
	
	// No unexplored POIs nearby - explore randomly
	return randomExplorationTarget(state)
}

// randomExplorationTarget picks a random direction to explore
func randomExplorationTarget(state *ExplorerState) *explorationTarget {
	angle := rand.Float64() * 2 * math.Pi
	distance := 150 + rand.Float64()*350 // 150-500m
	
	// Convert to lat/lon offset (approximate)
	latOffset := (distance / 111000) * math.Cos(angle)
	lonOffset := (distance / (111000 * math.Cos(state.CurrentLat*math.Pi/180))) * math.Sin(angle)
	
	return &explorationTarget{
		Lat:  state.CurrentLat + latOffset,
		Lon:  state.CurrentLon + lonOffset,
		Name: fmt.Sprintf("exploration %.0fm %s", distance, compassDirection(angle)),
	}
}

func compassDirection(radians float64) string {
	degrees := radians * 180 / math.Pi
	if degrees < 0 {
		degrees += 360
	}
	directions := []string{"N", "NE", "E", "SE", "S", "SW", "W", "NW"}
	index := int((degrees + 22.5) / 45) % 8
	return directions[index]
}

// indexPOIsAlongRoute queries OSM for POIs near route points
func indexPOIsAlongRoute(db *DB, agentID string, coords [][]float64) {
	// Sample points along route (every ~100m worth of points)
	step := len(coords) / 5
	if step < 1 {
		step = 1
	}
	
	indexed := 0
	for i := 0; i < len(coords); i += step {
		lon, lat := coords[i][0], coords[i][1] // OSRM returns [lon, lat]
		
		// Check if we've already indexed this area
		existing := db.Query(lat, lon, 50, EntityPlace, 1)
		if len(existing) > 0 {
			continue // Already indexed
		}
		
		// Query OSM for POIs (small radius to be efficient)
		pois := queryOSMPOIsNearby(lat, lon, 100, agentID)
		for _, poi := range pois {
			db.Insert(poi)
			indexed++
		}
	}
	
	if indexed > 0 {
		log.Printf("[explorer] indexed %d POIs along route", indexed)
	}
}

// queryOSMPOIsNearby queries OpenStreetMap for POIs near a point
func queryOSMPOIsNearby(lat, lon, radiusM float64, agentID string) []*Entity {
	query := fmt.Sprintf(`[out:json][timeout:10];
	(
		node["amenity"](around:%f,%f,%f);
		node["shop"](around:%f,%f,%f);
	);
	out body;`, radiusM, lat, lon, radiusM, lat, lon)
	
	apiURL := "https://overpass-api.de/api/interpreter?data=" + url.QueryEscape(query)
	resp, err := OSMGet(apiURL)
	if err != nil {
		log.Printf("[explorer] OSM query failed: %v", err)
		return nil
	}
	defer resp.Body.Close()
	
	var result struct {
		Elements []struct {
			ID   int64             `json:"id"`
			Lat  float64           `json:"lat"`
			Lon  float64           `json:"lon"`
			Tags map[string]string `json:"tags"`
		} `json:"elements"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil
	}
	
	var entities []*Entity
	for _, el := range result.Elements {
		name := el.Tags["name"]
		if name == "" {
			continue
		}
		
		category := el.Tags["amenity"]
		if category == "" {
			category = el.Tags["shop"]
		}
		
		entities = append(entities, &Entity{
			ID:   GenerateID(EntityPlace, el.Lat, el.Lon, name),
			Type: EntityPlace,
			Name: name,
			Lat:  el.Lat,
			Lon:  el.Lon,
			Data: &PlaceData{
				Category: category,
				Tags:     el.Tags,
				AgentID:  agentID,
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		})
	}
	
	return entities
}

func distanceMeters(lat1, lon1, lat2, lon2 float64) float64 {
	// Haversine formula
	R := 6371000.0 // Earth radius in meters
	phi1 := lat1 * math.Pi / 180
	phi2 := lat2 * math.Pi / 180
	deltaPhi := (lat2 - lat1) * math.Pi / 180
	deltaLambda := (lon2 - lon1) * math.Pi / 180
	
	a := math.Sin(deltaPhi/2)*math.Sin(deltaPhi/2) +
		math.Cos(phi1)*math.Cos(phi2)*math.Sin(deltaLambda/2)*math.Sin(deltaLambda/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	
	return R * c
}

// ExplorationMode controls whether agents explore
var ExplorationMode = false

// EnableExploration turns on exploration for an agent
func EnableExploration(agentID string) {
	if state := GetExplorerState(agentID); state != nil {
		state.Exploring = true
	}
}

// DisableExploration turns off exploration for an agent
func DisableExploration(agentID string) {
	if state := GetExplorerState(agentID); state != nil {
		state.Exploring = false
	}
}

// GetExplorerStats returns exploration statistics (from both memory and persisted data)
func GetExplorerStats() map[string]interface{} {
	stats := make(map[string]interface{})
	
	// Count from in-memory state
	memorySteps := 0
	for _, state := range explorerStates {
		memorySteps += state.TotalSteps
	}
	
	// Also count from persisted agent data (for agents not yet in memory)
	persistedSteps := 0
	db := Get()
	agents := db.ListAgents()
	for _, agent := range agents {
		// Skip if already counted in memory
		if _, ok := explorerStates[agent.ID]; ok {
			continue
		}
		if ad := agent.GetAgentData(); ad != nil {
			persistedSteps += ad.TotalSteps
		}
	}
	
	stats["exploring_agents"] = len(explorerStates)
	stats["total_steps"] = memorySteps + persistedSteps
	
	return stats
}

// GetAgentExplorationStats returns exploration stats for a specific agent (from memory or persisted)
func GetAgentExplorationStats(agent *Entity) (totalSteps int, homeLat, homeLon float64, distFromHome float64) {
	// Check in-memory state first
	if state := GetExplorerState(agent.ID); state != nil {
		return state.TotalSteps, state.HomeLat, state.HomeLon, DistanceFromHome(state)
	}
	
	// Fall back to persisted data
	if ad := agent.GetAgentData(); ad != nil && ad.TotalSteps > 0 {
		dist := distanceMeters(agent.Lat, agent.Lon, ad.HomeLat, ad.HomeLon)
		return ad.TotalSteps, ad.HomeLat, ad.HomeLon, dist
	}
	
	return 0, 0, 0, 0
}

// DistanceFromHome returns how far an explorer is from its starting point
func DistanceFromHome(state *ExplorerState) float64 {
	return distanceMeters(state.CurrentLat, state.CurrentLon, state.HomeLat, state.HomeLon)
}
