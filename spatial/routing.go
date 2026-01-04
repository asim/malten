package spatial

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const osrmBaseURL = "https://router.project-osrm.org"

type RouteStep struct {
	Instruction string
	Distance    float64 // meters
	Duration    float64 // seconds
	Name        string
}

type Route struct {
	Steps       []RouteStep
	TotalDist   float64 // meters
	TotalTime   float64 // seconds
	Summary     string
}

// GetWalkingDirections returns turn-by-turn walking directions
func GetWalkingDirections(fromLat, fromLon, toLat, toLon float64) (*Route, error) {
	url := fmt.Sprintf("%s/route/v1/foot/%f,%f;%f,%f?overview=false&steps=true",
		osrmBaseURL, fromLon, fromLat, toLon, toLat)
	
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Malten/1.0")
	
	resp, err := httpClient.Do(req)
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
			Legs     []struct {
				Steps []struct {
					Maneuver struct {
						Type     string `json:"type"`
						Modifier string `json:"modifier"`
					} `json:"maneuver"`
					Name     string  `json:"name"`
					Distance float64 `json:"distance"`
					Duration float64 `json:"duration"`
				} `json:"steps"`
			} `json:"legs"`
		} `json:"routes"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode failed: %v", err)
	}
	
	if data.Code != "Ok" || len(data.Routes) == 0 {
		return nil, fmt.Errorf("no route found")
	}
	
	route := data.Routes[0]
	result := &Route{
		TotalDist: route.Distance,
		TotalTime: route.Duration,
	}
	
	// Combine short steps, skip trivial ones
	for _, leg := range route.Legs {
		for _, step := range leg.Steps {
			if step.Distance < 5 { // Skip very short steps
				continue
			}
			
			instruction := formatInstruction(step.Maneuver.Type, step.Maneuver.Modifier, step.Name)
			if instruction == "" {
				continue
			}
			
			result.Steps = append(result.Steps, RouteStep{
				Instruction: instruction,
				Distance:    step.Distance,
				Duration:    step.Duration,
				Name:        step.Name,
			})
		}
	}
	
	// Build summary
	result.Summary = formatRouteSummary(result.TotalDist, result.TotalTime)
	
	return result, nil
}

func formatInstruction(maneuverType, modifier, name string) string {
	if maneuverType == "arrive" {
		return "🏁 Arrive at destination"
	}
	if maneuverType == "depart" {
		if name != "" {
			return fmt.Sprintf("🚶 Start on %s", name)
		}
		return "🚶 Start walking"
	}
	
	// Direction emoji
	var dir string
	switch modifier {
	case "left":
		dir = "⬅️ Turn left"
	case "right":
		dir = "➡️ Turn right"
	case "slight left":
		dir = "↖️ Bear left"
	case "slight right":
		dir = "↗️ Bear right"
	case "sharp left":
		dir = "↩️ Sharp left"
	case "sharp right":
		dir = "↪️ Sharp right"
	case "straight":
		dir = "⬆️ Continue straight"
	case "uturn":
		dir = "🔄 U-turn"
	default:
		if maneuverType == "continue" {
			dir = "⬆️ Continue"
		} else {
			dir = "➡️ " + strings.Title(maneuverType)
		}
	}
	
	if name != "" && name != "unnamed road" {
		return fmt.Sprintf("%s onto %s", dir, name)
	}
	return dir
}

func formatRouteSummary(distMeters, durSeconds float64) string {
	var dist string
	if distMeters >= 1000 {
		dist = fmt.Sprintf("%.1f km", distMeters/1000)
	} else {
		dist = fmt.Sprintf("%.0f m", distMeters)
	}
	
	// Calculate walking time at 5 km/h (~83 m/min) instead of trusting OSRM
	// OSRM demo server returns driving-speed times even for foot profile
	mins := int(distMeters / 83)
	if mins < 1 {
		return fmt.Sprintf("🚶 %s · less than 1 min", dist)
	} else if mins < 60 {
		return fmt.Sprintf("🚶 %s · %d min walk", dist, mins)
	} else {
		hours := mins / 60
		mins = mins % 60
		return fmt.Sprintf("🚶 %s · %dh %dm walk", dist, hours, mins)
	}
}

// FormatDirections returns a formatted string of walking directions
func FormatDirections(route *Route) string {
	return FormatDirectionsWithMap(route, 0, 0, 0, 0, "")
}

// FormatDirectionsWithMap returns directions with a Google Maps link
func FormatDirectionsWithMap(route *Route, fromLat, fromLon, toLat, toLon float64, destName string) string {
	if route == nil {
		return "No route found"
	}
	
	// Very short distance - you're basically there
	if route.TotalDist < 50 {
		return fmt.Sprintf("You're already there! (%.0fm away)", route.TotalDist)
	}
	
	if len(route.Steps) == 0 {
		return "No route found"
	}
	
	var lines []string
	lines = append(lines, route.Summary)
	lines = append(lines, "")
	
	// Add all steps
	for i, step := range route.Steps {
		distStr := ""
		if step.Distance >= 100 {
			distStr = fmt.Sprintf(" (%.0fm)", step.Distance)
		}
		lines = append(lines, fmt.Sprintf("%d. %s%s", i+1, step.Instruction, distStr))
	}
	
	// Add Google Maps directions link
	if fromLat != 0 && toLat != 0 {
		mapURL := fmt.Sprintf("https://www.google.com/maps/dir/?api=1&origin=%.6f,%.6f&destination=%.6f,%.6f&travelmode=walking",
			fromLat, fromLon, toLat, toLon)
		lines = append(lines, "")
		lines = append(lines, "🗺️ "+mapURL)
	}
	
	return strings.Join(lines, "\n")
}

// SearchOSM searches for a place by name near a location
func SearchOSM(query string, nearLat, nearLon float64) ([]*Entity, error) {
	// Use Nominatim search with viewbox to prefer nearby results
	url := fmt.Sprintf(
		"https://nominatim.openstreetmap.org/search?q=%s&format=json&limit=5&viewbox=%f,%f,%f,%f&bounded=0",
		query,
		nearLon-0.1, nearLat+0.1, nearLon+0.1, nearLat-0.1,
	)
	
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Malten/1.0")
	
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var results []struct {
		DisplayName string  `json:"display_name"`
		Lat         string  `json:"lat"`
		Lon         string  `json:"lon"`
		Type        string  `json:"type"`
		Class       string  `json:"class"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, err
	}
	
	var entities []*Entity
	for _, r := range results {
		var lat, lon float64
		fmt.Sscanf(r.Lat, "%f", &lat)
		fmt.Sscanf(r.Lon, "%f", &lon)
		
		// Extract short name from display_name (first part before comma)
		name := r.DisplayName
		if idx := strings.Index(name, ","); idx > 0 {
			name = name[:idx]
		}
		
		entities = append(entities, &Entity{
			Type: EntityPlace,
			Name: name,
			Lat:  lat,
			Lon:  lon,
			Data: map[string]interface{}{
				"type":         r.Type,
				"class":        r.Class,
				"display_name": r.DisplayName,
			},
		})
	}
	
	return entities, nil
}
