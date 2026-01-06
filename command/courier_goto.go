package command

import (
	"fmt"
	"strings"

	"malten.ai/spatial"
)

func init() {
	Register(&Command{
		Name:        "goto",
		Description: "Send courier to a specific place",
		Usage:       "/goto <place name> or /goto <lat>,<lon>",
		Handler:     handleGoto,
	})
}

func handleGoto(ctx *Context, args []string) (string, error) {
	if len(args) == 0 {
		return "Usage: /goto <place name> or /goto <lat>,<lon>", nil
	}

	destination := strings.Join(args, " ")
	
	// Check if it's coordinates
	var destLat, destLon float64
	var destName string
	
	if strings.Contains(destination, ",") {
		// Try to parse as coordinates
		parts := strings.Split(destination, ",")
		if len(parts) == 2 {
			var lat, lon float64
			_, err := fmt.Sscanf(strings.TrimSpace(parts[0]), "%f", &lat)
			if err == nil {
				_, err = fmt.Sscanf(strings.TrimSpace(parts[1]), "%f", &lon)
				if err == nil {
					destLat = lat
					destLon = lon
					destName = fmt.Sprintf("%.4f, %.4f", lat, lon)
				}
			}
		}
	}
	
	// If not coordinates, search for place
	if destName == "" {
		db := spatial.Get()
		
		// First try nearby places if we have user location
		if ctx.Lat != 0 || ctx.Lon != 0 {
			places := db.Query(ctx.Lat, ctx.Lon, 10000, spatial.EntityPlace, 100)
			for _, place := range places {
				if strings.Contains(strings.ToLower(place.Name), strings.ToLower(destination)) {
					destLat = place.Lat
					destLon = place.Lon
					destName = place.Name
					break
				}
			}
		}
		
		// If still not found, return error
		if destName == "" {
			return fmt.Sprintf("❌ Couldn't find '%s'. Try coordinates: /goto 51.41,-0.30", destination), nil
		}
	}
	
	// Send courier there
	err := spatial.SendCourierTo(destLat, destLon, destName)
	if err != nil {
		return fmt.Sprintf("❌ Courier error: %v", err), nil
	}
	
	return fmt.Sprintf("🚴 Courier heading to **%s** (%.4f, %.4f)", destName, destLat, destLon), nil
}
