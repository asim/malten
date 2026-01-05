package server

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"strconv"

	"malten.ai/spatial"
)

//go:embed map.html
var mapHTML []byte

func serveMapHTML(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Write(mapHTML)
}

// MapDataResponse contains spatial data for map rendering
type MapDataResponse struct {
	Bounds     *Bounds      `json:"bounds"`
	Agents     []MapAgent   `json:"agents"`
	Places     []MapPlace   `json:"places"`
	Streets    []MapStreet  `json:"streets,omitempty"`
	Weather    []MapWeather `json:"weather,omitempty"`
}

type Bounds struct {
	MinLat float64 `json:"minLat"`
	MaxLat float64 `json:"maxLat"`
	MinLon float64 `json:"minLon"`
	MaxLon float64 `json:"maxLon"`
}

type MapAgent struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
	Radius   float64 `json:"radius"`
	Status   string  `json:"status"`
	POICount int     `json:"poiCount"`
}

type MapPlace struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Category string  `json:"category"`
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
}

type MapWeather struct {
	Lat       float64 `json:"lat"`
	Lon       float64 `json:"lon"`
	Temp      float64 `json:"temp"`
	Condition string  `json:"condition"`
}

type MapStreet struct {
	ID     string      `json:"id"`
	Name   string      `json:"name"`
	Points [][]float64 `json:"points"` // [[lon, lat], ...]
	Length float64     `json:"length"`
}

// MapHandler handles GET /map
// Returns HTML map view by default, or JSON data if Accept: application/json
func MapHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check Accept header - serve JSON if requested
	accept := r.Header.Get("Accept")
	if accept != "application/json" {
		// Serve HTML map view
		serveMapHTML(w, r)
		return
	}

	// Parse optional query params for bounds filtering
	latStr := r.URL.Query().Get("lat")
	lonStr := r.URL.Query().Get("lon")
	radiusStr := r.URL.Query().Get("radius")

	var centerLat, centerLon, radius float64
	if latStr != "" && lonStr != "" {
		centerLat, _ = strconv.ParseFloat(latStr, 64)
		centerLon, _ = strconv.ParseFloat(lonStr, 64)
		radius = 10000 // Default 10km
		if radiusStr != "" {
			radius, _ = strconv.ParseFloat(radiusStr, 64)
		}
	} else {
		// Default to London area if no center specified
		centerLat = 51.45
		centerLon = -0.35
		radius = 20000 // 20km
	}

	db := spatial.Get()

	// Get all agents
	agents := db.ListAgents()
	mapAgents := make([]MapAgent, 0, len(agents))
	for _, a := range agents {
		var status string
		var poiCount int
		agentRadius := 5000.0
		if agentData := a.GetAgentData(); agentData != nil {
			status = agentData.Status
			poiCount = agentData.POICount
			if agentData.Radius > 0 {
				agentRadius = agentData.Radius
			}
		}
		mapAgents = append(mapAgents, MapAgent{
			ID:       a.ID,
			Name:     a.Name,
			Lat:      a.Lat,
			Lon:      a.Lon,
			Radius:   agentRadius,
			Status:   status,
			POICount: poiCount,
		})
	}

	// Get places within radius
	places := db.Query(centerLat, centerLon, radius, spatial.EntityPlace, 5000)
	mapPlaces := make([]MapPlace, 0, len(places))
	for _, p := range places {
		if p.Name == "" {
			continue // Skip unnamed places
		}
		var category string
		if placeData := p.GetPlaceData(); placeData != nil {
			category = placeData.Category
		}
		mapPlaces = append(mapPlaces, MapPlace{
			ID:       p.ID,
			Name:     p.Name,
			Category: category,
			Lat:      p.Lat,
			Lon:      p.Lon,
		})
	}

	// Get weather data
	weatherEntities := db.Query(centerLat, centerLon, radius, spatial.EntityWeather, 100)
	mapWeather := make([]MapWeather, 0, len(weatherEntities))
	for _, w := range weatherEntities {
		var temp float64
		if wd := w.GetWeatherData(); wd != nil {
			temp = wd.TempC
		}
		mapWeather = append(mapWeather, MapWeather{
			Lat:       w.Lat,
			Lon:       w.Lon,
			Temp:      temp,
			Condition: w.Name, // Use Name which contains formatted condition
		})
	}

	// Get street data
	streetEntities := db.Query(centerLat, centerLon, radius, spatial.EntityStreet, 500)
	mapStreets := make([]MapStreet, 0, len(streetEntities))
	for _, s := range streetEntities {
		var convertedPoints [][]float64
		var length float64
		
		if sd := s.GetStreetData(); sd != nil {
			convertedPoints = sd.Points
			length = sd.Length
		} else if m, ok := s.Data.(map[string]interface{}); ok {
			// Legacy fallback
			length, _ = m["length"].(float64)
			if points, ok := m["points"].([]interface{}); ok {
				convertedPoints = make([][]float64, 0, len(points))
				for _, p := range points {
					if pt, ok := p.([]interface{}); ok && len(pt) >= 2 {
						lon, _ := pt[0].(float64)
						lat, _ := pt[1].(float64)
						convertedPoints = append(convertedPoints, []float64{lon, lat})
					}
				}
			}
		}
		
		if len(convertedPoints) < 2 {
			continue
		}
		
		mapStreets = append(mapStreets, MapStreet{
			ID:     s.ID,
			Name:   s.Name,
			Points: convertedPoints,
			Length: length,
		})
	}

	// Calculate bounds from data
	var bounds *Bounds
	if len(mapPlaces) > 0 {
		minLat, maxLat := mapPlaces[0].Lat, mapPlaces[0].Lat
		minLon, maxLon := mapPlaces[0].Lon, mapPlaces[0].Lon
		for _, p := range mapPlaces {
			if p.Lat < minLat {
				minLat = p.Lat
			}
			if p.Lat > maxLat {
				maxLat = p.Lat
			}
			if p.Lon < minLon {
				minLon = p.Lon
			}
			if p.Lon > maxLon {
				maxLon = p.Lon
			}
		}
		bounds = &Bounds{
			MinLat: minLat,
			MaxLat: maxLat,
			MinLon: minLon,
			MaxLon: maxLon,
		}
	}

	response := MapDataResponse{
		Bounds:  bounds,
		Agents:  mapAgents,
		Places:  mapPlaces,
		Streets: mapStreets,
		Weather: mapWeather,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
