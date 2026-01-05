package spatial

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"time"
)

// haversineDistance calculates distance in meters between two points
func haversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000 // Earth radius in meters
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// Street represents a segment of road with its geometry
type Street struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Points    [][]float64 `json:"points"` // [[lon, lat], ...]
	Length    float64     `json:"length"` // meters
	FetchedAt time.Time   `json:"fetched_at"`
}

// FetchStreetGeometry fetches the street geometry between two points from OSRM
// Returns the decoded polyline as a series of [lon, lat] coordinates
func FetchStreetGeometry(fromLat, fromLon, toLat, toLon float64) (*Street, error) {
	// Use OSRM with full geometry
	url := fmt.Sprintf("%s/route/v1/foot/%f,%f;%f,%f?overview=full&geometries=geojson",
		osrmBaseURL, fromLon, fromLat, toLon, toLat)

	resp, err := OSRMGet(url)
	if err != nil {
		return nil, fmt.Errorf("OSRM request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("OSRM returned %d", resp.StatusCode)
	}

	var data struct {
		Code   string `json:"code"`
		Routes []struct {
			Distance float64 `json:"distance"`
			Geometry struct {
				Type        string      `json:"type"`
				Coordinates [][]float64 `json:"coordinates"` // [[lon, lat], ...]
			} `json:"geometry"`
			Legs []struct {
				Steps []struct {
					Name string `json:"name"`
				} `json:"steps"`
			} `json:"legs"`
		} `json:"routes"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("OSRM decode failed: %v", err)
	}

	if data.Code != "Ok" || len(data.Routes) == 0 {
		return nil, fmt.Errorf("no route found")
	}

	route := data.Routes[0]

	// Extract street names from steps
	var streetName string
	for _, leg := range route.Legs {
		for _, step := range leg.Steps {
			if step.Name != "" && step.Name != "unnamed road" {
				streetName = step.Name
				break
			}
		}
		if streetName != "" {
			break
		}
	}

	// Generate ID from both endpoints to ensure uniqueness
	idStr := fmt.Sprintf("%f,%f->%f,%f", fromLat, fromLon, toLat, toLon)
	return &Street{
		ID:        GenerateID(EntityStreet, fromLat, fromLon, idStr),
		Name:      streetName,
		Points:    route.Geometry.Coordinates,
		Length:    route.Distance,
		FetchedAt: time.Now(),
	}, nil
}

// IndexStreetsAroundAgent fetches street geometry in the agent's area
// by querying routes between the agent center and nearby POIs
// maxRoutes limits how many routes to fetch (0 = all)
func IndexStreetsAroundAgent(agent *Entity, maxRoutes int) int {
	db := Get()
	radius := 2000.0 // 2km

	// Get nearby places to use as route endpoints
	places := db.Query(agent.Lat, agent.Lon, radius, EntityPlace, 50)
	if len(places) < 2 {
		return 0
	}

	// Get existing streets to avoid duplicates
	existingStreets := db.Query(agent.Lat, agent.Lon, radius, EntityStreet, 200)
	existingRoutes := make(map[string]bool)
	for _, s := range existingStreets {
		toName, _ := s.Data["to_name"].(string)
		existingRoutes[toName] = true
	}

	var count int

	// Fetch routes from agent center to each nearby place
	for _, place := range places {
		if maxRoutes > 0 && count >= maxRoutes {
			break
		}

		if existingRoutes[place.Name] {
			continue
		}

		// Skip places too close (< 200m) - we want meaningful routes
		dist := haversineDistance(agent.Lat, agent.Lon, place.Lat, place.Lon)
		if dist < 200 {
			continue
		}

		street, err := FetchStreetGeometry(agent.Lat, agent.Lon, place.Lat, place.Lon)
		if err != nil {
			log.Printf("[streets] Failed to fetch route to %s: %v", place.Name, err)
			continue
		}

		if len(street.Points) < 2 {
			continue
		}

		// Store the route
		// Use the midpoint for spatial indexing
		midIdx := len(street.Points) / 2
		midLon := street.Points[midIdx][0]
		midLat := street.Points[midIdx][1]

		entity := &Entity{
			ID:   street.ID,
			Type: EntityStreet,
			Name: street.Name,
			Lat:  midLat,
			Lon:  midLon,
			Data: map[string]interface{}{
				"points":   street.Points,
				"length":   street.Length,
				"agent_id": agent.ID,
				"from_lat": agent.Lat,
				"from_lon": agent.Lon,
				"to_lat":   place.Lat,
				"to_lon":   place.Lon,
				"to_name":  place.Name,
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		db.Insert(entity)
		count++
		existingRoutes[place.Name] = true

		log.Printf("[streets] Indexed route to %s (%d points, %.0fm)", place.Name, len(street.Points), street.Length)
	}

	return count
}

// IndexStreetsAsync starts background street indexing for an agent
func IndexStreetsAsync(agent *Entity) {
	go func() {
		count := IndexStreetsAroundAgent(agent, 0) // No limit in background
		log.Printf("[streets] Background indexing complete for %s: %d routes", agent.Name, count)
	}()
}
