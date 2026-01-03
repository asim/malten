package command

import (
	"fmt"
	"log"
	"strings"

	"malten.ai/spatial"
)

// Directions handles "how do I get to X" type questions
// DirectionsTo returns walking directions to a known destination
func DirectionsTo(destName string, fromLat, fromLon, toLat, toLon float64) (string, error) {
	if fromLat == 0 && fromLon == 0 {
		return "📍 Need your location for directions. Enable location?", nil
	}
	
	route, err := spatial.GetWalkingDirections(fromLat, fromLon, toLat, toLon)
	if err != nil {
		return fmt.Sprintf("🚶 Couldn't get directions to %s: %v", destName, err), nil
	}
	
	return fmt.Sprintf("🚶 Walking to %s\n\n%s", destName,
		spatial.FormatDirectionsWithMap(route, fromLat, fromLon, toLat, toLon, destName)), nil
}

func Directions(destination string, fromLat, fromLon float64) (string, error) {
	if fromLat == 0 && fromLon == 0 {
		return "📍 Need your location for directions. Enable location?", nil
	}
	
	// Clean up destination
	destination = strings.TrimSpace(destination)
	destination = strings.TrimPrefix(destination, "to ")
	destination = strings.TrimPrefix(destination, "the ")
	
	var destLat, destLon float64
	var destName string
	db := spatial.Get()
	
	// Handle generic terms - find nearest from spatial DB
	lower := strings.ToLower(destination)
	switch {
	case lower == "bus" || lower == "bus stop" || lower == "nearest bus" || lower == "closest bus":
		// Find nearest bus stop from arrivals cache
		arrivals := db.Query(fromLat, fromLon, 1000, spatial.EntityArrival, 1)
		if len(arrivals) > 0 {
			destLat, destLon = arrivals[0].Lat, arrivals[0].Lon
			if name, ok := arrivals[0].Data["stop_name"].(string); ok {
				destName = name
			} else {
				destName = "bus stop"
			}
		}
	case lower == "station" || lower == "train station" || lower == "rail station" || lower == "nearest station":
		// Find nearest rail station from our quadtree first
		log.Printf("[directions] looking for nearest station in quadtree")
		stations := db.QueryByNameContains(fromLat, fromLon, 5000, "Station")
		for _, s := range stations {
			// Skip bus stops (they have 🚏 or "Bus" in name usually)
			if strings.Contains(s.Name, "Bus") || s.Type == spatial.EntityArrival {
				continue
			}
			log.Printf("[directions] found station: %s at %.4f,%.4f", s.Name, s.Lat, s.Lon)
			destLat, destLon = s.Lat, s.Lon
			destName = s.Name
			break
		}
		// Fallback to geocoding if nothing in quadtree
		if destLat == 0 {
			lat, lon, err := GeocodeNear("railway station", fromLat, fromLon)
			if err == nil {
				destLat, destLon = lat, lon
				destName = "station"
			}
		}
	}
	
	// Try our quadtree first - we may have this place indexed
	if destLat == 0 {
		log.Printf("[directions] searching quadtree for %q", destination)
		results := db.QueryByNameContains(fromLat, fromLon, 5000, destination)
		for _, r := range results {
			// Skip arrival entities (bus times) - they may have user's location not stop's
			if r.Type == spatial.EntityArrival {
				continue
			}
			log.Printf("[directions] found in quadtree: %s (type=%s) at %.4f,%.4f", r.Name, r.Type, r.Lat, r.Lon)
			destLat, destLon = r.Lat, r.Lon
			destName = r.Name
			break
		}
	}
	
	// Fallback to geocoding
	if destLat == 0 {
		log.Printf("[directions] geocoding %q near %.4f,%.4f", destination, fromLat, fromLon)
		lat, lon, err := GeocodeNear(destination, fromLat, fromLon)
		log.Printf("[directions] geocode result: %.4f,%.4f err=%v", lat, lon, err)
		if err == nil {
			destLat, destLon = lat, lon
			destName = destination
		}
	}
	
	// Still nothing? Try OSM search
	if destLat == 0 {
		results, _ := spatial.SearchOSM(destination, fromLat, fromLon)
		if len(results) > 0 {
			destLat, destLon = results[0].Lat, results[0].Lon
			destName = results[0].Name
		}
	}
	
	if destLat == 0 {
		return fmt.Sprintf("📍 Couldn't find '%s'. Try being more specific?", destination), nil
	}
	
	// Get walking directions
	route, err := spatial.GetWalkingDirections(fromLat, fromLon, destLat, destLon)
	if err != nil {
		return fmt.Sprintf("🚶 Couldn't get directions to %s: %v", destName, err), nil
	}
	
	result := fmt.Sprintf("🚶 Walking to %s\n\n%s", destName, 
		spatial.FormatDirectionsWithMap(route, fromLat, fromLon, destLat, destLon, destName))
	return result, nil
}

func init() {
	Register(&Command{
		Name:        "directions",
		Description: "Get walking directions to a place",
		Usage:       "/directions <place name>",
		Handler: func(ctx *Context, args []string) (string, error) {
			if len(args) == 0 {
				return "Usage: /directions <place name>", nil
			}
			destination := strings.Join(args, " ")
			
			// If destination coords provided, use them directly
			if ctx.ToLat != 0 && ctx.ToLon != 0 {
				return DirectionsTo(destination, ctx.Lat, ctx.Lon, ctx.ToLat, ctx.ToLon)
			}
			
			return Directions(destination, ctx.Lat, ctx.Lon)
		},
	})
}
