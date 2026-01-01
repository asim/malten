package spatial

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
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

// CreateAgent creates a new agent
func (d *DB) CreateAgent(lat, lon, radius float64, name string) *Entity {
	agent := &Entity{
		ID:   GenerateID(EntityAgent, lat, lon, name),
		Type: EntityAgent,
		Name: name,
		Lat:  lat,
		Lon:  lon,
		Data: map[string]interface{}{
			"radius":     radius,
			"status":     "active",
			"poi_count":  0,
			"last_index": nil,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	d.Insert(agent)
	return agent
}

// FindOrCreateAgent finds or creates an agent for a location
func (d *DB) FindOrCreateAgent(lat, lon float64) *Entity {
	agent := d.FindAgent(lat, lon, AgentRadius)
	if agent != nil {
		return agent
	}

	areaName := ReverseGeocode(lat, lon)
	if areaName == "" {
		areaName = fmt.Sprintf("Area %.2f,%.2f", lat, lon)
	}

	agent = d.CreateAgent(lat, lon, AgentRadius, areaName)
	go IndexAgent(agent)
	return agent
}

// recoverStaleAgents resumes indexing for agents stuck in indexing state
func (d *DB) recoverStaleAgents() {
	agents := d.ListAgents()
	for _, agent := range agents {
		if status, _ := agent.Data["status"].(string); status == "indexing" {
			log.Printf("[spatial] Resuming indexing for %s", agent.Name)
			go IndexAgent(agent)
		}
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
		"amenity=cafe", "amenity=restaurant", "amenity=pharmacy",
		"amenity=hospital", "amenity=bank", "amenity=fuel",
		"amenity=place_of_worship", "shop=supermarket",
		"amenity=parking", "tourism=hotel",
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

	// Extract category name
	cat := category
	if strings.HasPrefix(category, "amenity=") {
		cat = category[8:]
	} else if strings.HasPrefix(category, "shop=") {
		cat = category[5:]
	} else if strings.HasPrefix(category, "tourism=") {
		cat = category[8:]
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
