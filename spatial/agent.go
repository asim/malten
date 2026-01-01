package spatial

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	AgentRadius    = 5000.0 // 5km
	OSMRateLimit   = 5 * time.Second
	OverpassURL    = "https://overpass-api.de/api/interpreter"
	NominatimURL   = "https://nominatim.openstreetmap.org"
)

// FindAgent finds an agent covering the location
func (d *DB) FindAgent(lat, lon, radius float64) *Entity {
	agents := d.Query(lat, lon, radius, EntityAgent, 1)
	if len(agents) > 0 {
		return agents[0]
	}
	return nil
}

// ListAgents returns all agents
func (d *DB) ListAgents() []*Entity {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var agents []*Entity
	for _, p := range d.entities {
		if entity, ok := p.Data().(*Entity); ok && entity.Type == EntityAgent {
			agents = append(agents, entity)
		}
	}
	return agents
}

// DefaultAgentPrompt defines what an agent indexes
const DefaultAgentPrompt = `You are a spatial indexer for this area.
Index and maintain:
- Static places (cafes, restaurants, pharmacies, shops)
- Live transport (bus arrivals, train times)
- Weather conditions
- Prayer times
Update live data every 30 seconds. Re-index static POIs daily.`

// CreateAgent creates a new agent with a prompt
func (d *DB) CreateAgent(lat, lon, radius float64, name string) *Entity {
	agent := &Entity{
		ID:   GenerateID(EntityAgent, lat, lon, name),
		Type: EntityAgent,
		Name: name,
		Lat:  lat,
		Lon:  lon,
		Data: map[string]interface{}{
			"radius":      radius,
			"status":      "active",
			"prompt":      DefaultAgentPrompt,
			"poi_count":   0,
			"last_index":  nil,
			"last_live":   nil,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	d.Insert(agent)
	return agent
}

// FindOrCreateAgent finds or creates an agent for a location
func (d *DB) FindOrCreateAgent(lat, lon float64) *Entity {
	areaName := ReverseGeocode(lat, lon)
	if areaName == "" {
		areaName = fmt.Sprintf("Area %.2f,%.2f", lat, lon)
	}
	return d.FindOrCreateAgentNamed(lat, lon, areaName)
}

// FindOrCreateAgentNamed finds agent by name or creates one
func (d *DB) FindOrCreateAgentNamed(lat, lon float64, name string) *Entity {
	// Check if agent with this name exists
	for _, a := range d.ListAgents() {
		if a.Name == name {
			return a
		}
	}

	agent := d.CreateAgent(lat, lon, AgentRadius, name)
	StartAgentLoop(agent)
	return agent
}

// Track running agent loops
var runningAgents = make(map[string]bool)
var runningAgentsMu sync.Mutex

// StartAgentLoop starts the continuous agent loop
func StartAgentLoop(agent *Entity) {
	runningAgentsMu.Lock()
	if runningAgents[agent.ID] {
		runningAgentsMu.Unlock()
		return // Already running
	}
	runningAgents[agent.ID] = true
	runningAgentsMu.Unlock()
	
	go agentLoop(agent)
}

func agentLoop(agent *Entity) {
	log.Printf("[agent] %s started", agent.Name)
	
	// Start live data immediately (don't wait for POI index)
	go func() {
		updateLiveData(agent)
		
		// Continuous live data loop
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		
		for range ticker.C {
			updateLiveData(agent)
		}
	}()
	
	// POI index runs in parallel (takes longer)
	IndexAgent(agent)
}

func updateLiveData(agent *Entity) {
	db := Get()
	
	// Location name (reverse geocode) - only if not cached
	if loc := fetchLocation(agent.Lat, agent.Lon); loc != nil {
		db.Insert(loc)
	}
	
	// Weather
	if weather := fetchWeather(agent.Lat, agent.Lon); weather != nil {
		db.Insert(weather)
	}
	
	// Prayer times
	if prayer := fetchPrayerTimes(agent.Lat, agent.Lon); prayer != nil {
		db.Insert(prayer)
	}
	
	// Transport arrivals (buses, tubes, trains)
	var totalArrivals int
	
	// Buses
	busArrivals := fetchTransportArrivals(agent.Lat, agent.Lon, "NaptanPublicBusCoachTram", "🚌")
	if len(busArrivals) > 0 {
		for _, arr := range busArrivals {
			db.Insert(arr)
		}
		totalArrivals += len(busArrivals)
	} else {
		// API returned nothing - extend existing arrivals TTL
		db.ExtendArrivalsTTL(agent.Lat, agent.Lon, 500)
	}
	
	// Tube stations
	tubeArrivals := fetchTransportArrivals(agent.Lat, agent.Lon, "NaptanMetroStation", "🚇")
	if len(tubeArrivals) > 0 {
		for _, arr := range tubeArrivals {
			db.Insert(arr)
		}
		totalArrivals += len(tubeArrivals)
	}
	
	// Rail stations (Overground, National Rail)
	railArrivals := fetchTransportArrivals(agent.Lat, agent.Lon, "NaptanRailStation", "🚆")
	if len(railArrivals) > 0 {
		for _, arr := range railArrivals {
			db.Insert(arr)
		}
		totalArrivals += len(railArrivals)
	}
	
	log.Printf("[agent] %s live update: %d arrivals", agent.Name, totalArrivals)
	
	// Update agent timestamp
	agent.Data["last_live"] = time.Now().Format(time.RFC3339)
	db.Insert(agent)
}

// recoverStaleAgents starts loops for all agents
func (d *DB) recoverStaleAgents() {
	agents := d.ListAgents()
	for _, agent := range agents {
		StartAgentLoop(agent)
	}
}

// IndexAgent indexes POIs in agent's territory
func IndexAgent(agent *Entity) {
	if agent == nil || agent.Type != EntityAgent {
		return
	}

	log.Printf("[spatial] Indexing %s", agent.Name)

	radius, _ := agent.Data["radius"].(float64)
	if radius == 0 {
		radius = AgentRadius
	}

	agent.Data["status"] = "indexing"
	Get().Insert(agent)

	categories := []string{
		// Food & drink
		"amenity=cafe", "amenity=restaurant", "amenity=fast_food",
		"amenity=pub", "amenity=bar",
		// Health
		"amenity=pharmacy", "amenity=hospital", "amenity=clinic",
		"amenity=dentist", "amenity=doctors",
		// Transport
		"railway=station", "railway=halt",
		"highway=bus_stop", "amenity=bus_station",
		"public_transport=station",
		// Services
		"amenity=bank", "amenity=atm", "amenity=post_office",
		"amenity=fuel", "amenity=parking",
		// Shopping
		"shop=supermarket", "shop=convenience", "shop=bakery",
		"shop=butcher", "shop=greengrocer",
		// Other
		"amenity=place_of_worship", "tourism=hotel",
		"leisure=park", "amenity=library",
	}

	var totalCount int
	for _, cat := range categories {
		count := indexCategory(agent, cat, radius)
		totalCount += count
		time.Sleep(OSMRateLimit)
	}

	log.Printf("[spatial] %s indexed %d POIs", agent.Name, totalCount)

	agent.Data["status"] = "active"
	agent.Data["poi_count"] = totalCount
	agent.Data["last_index"] = time.Now().Format(time.RFC3339)
	Get().Insert(agent)
}

func indexCategory(agent *Entity, category string, radius float64) int {
	query := fmt.Sprintf(`
[out:json][timeout:25];
(
  node[%s](around:%.0f,%f,%f);
  way[%s](around:%.0f,%f,%f);
);
out center 50;
`, category, radius, agent.Lat, agent.Lon, category, radius, agent.Lat, agent.Lon)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.PostForm(OverpassURL, url.Values{"data": {query}})
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return 0
	}

	var data struct {
		Elements []struct {
			Type   string            `json:"type"`
			ID     int64             `json:"id"`
			Lat    float64           `json:"lat"`
			Lon    float64           `json:"lon"`
			Center *struct {
				Lat float64 `json:"lat"`
				Lon float64 `json:"lon"`
			} `json:"center,omitempty"`
			Tags map[string]string `json:"tags"`
		} `json:"elements"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return 0
	}

	// Extract category name from tag=value format
	cat := category
	if idx := strings.Index(category, "="); idx > 0 {
		cat = category[idx+1:]
	}

	db := Get()
	for _, el := range data.Elements {
		lat, lon := el.Lat, el.Lon
		if lat == 0 && el.Center != nil {
			lat, lon = el.Center.Lat, el.Center.Lon
		}
		if lat == 0 && lon == 0 {
			continue
		}

		tags := make(map[string]interface{})
		for k, v := range el.Tags {
			tags[k] = v
		}

		entity := &Entity{
			Type: EntityPlace,
			Name: el.Tags["name"],
			Lat:  lat,
			Lon:  lon,
			Data: map[string]interface{}{
				"category": cat,
				"tags":     tags,
				"osm_id":   el.ID,
				"osm_type": el.Type,
				"agent_id": agent.ID,
			},
		}
		db.Insert(entity)
	}

	return len(data.Elements)
}

// ReverseGeocode gets area name from coordinates
func ReverseGeocode(lat, lon float64) string {
	client := &http.Client{Timeout: 5 * time.Second}
	url := fmt.Sprintf("%s/reverse?lat=%f&lon=%f&format=json&zoom=14", NominatimURL, lat, lon)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Malten/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	var result struct {
		Address struct {
			Suburb  string `json:"suburb"`
			Town    string `json:"town"`
			City    string `json:"city"`
			Village string `json:"village"`
		} `json:"address"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	if result.Address.Suburb != "" {
		return result.Address.Suburb
	}
	if result.Address.Town != "" {
		return result.Address.Town
	}
	if result.Address.Village != "" {
		return result.Address.Village
	}
	return result.Address.City
}
