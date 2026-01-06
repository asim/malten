package command

import (
	"fmt"
	"strings"

	"malten.ai/spatial"
)

func init() {
	Register(&Command{
		Name:        "route",
		Description: "Send courier through multiple waypoints",
		Usage:       "/route <lat,lon> <lat,lon> ... or /route clear",
		Handler:     handleRoute,
	})
}

func handleRoute(ctx *Context, args []string) (string, error) {
	if len(args) == 0 {
		// Show current queued waypoints
		waypoints := spatial.GetCourierWaypoints()
		if len(waypoints) == 0 {
			return "No waypoints queued. Usage: /route <lat,lon> <lat,lon> ...", nil
		}
		var lines []string
		lines = append(lines, fmt.Sprintf("📍 **%d waypoints queued:**", len(waypoints)))
		for i, wp := range waypoints {
			lines = append(lines, fmt.Sprintf("%d. %s (%.4f, %.4f)", i+1, wp.Name, wp.Lat, wp.Lon))
		}
		return strings.Join(lines, "\n"), nil
	}

	if args[0] == "clear" {
		spatial.ClearCourierWaypoints()
		return "🗑️ Waypoints cleared", nil
	}

	// Parse waypoints
	var waypoints []spatial.Waypoint
	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg == "" {
			continue
		}
		
		// Try to parse as coordinates
		parts := strings.Split(arg, ",")
		if len(parts) != 2 {
			return fmt.Sprintf("❌ Invalid waypoint '%s'. Use lat,lon format", arg), nil
		}
		
		var lat, lon float64
		_, err := fmt.Sscanf(strings.TrimSpace(parts[0]), "%f", &lat)
		if err != nil {
			return fmt.Sprintf("❌ Invalid latitude in '%s'", arg), nil
		}
		_, err = fmt.Sscanf(strings.TrimSpace(parts[1]), "%f", &lon)
		if err != nil {
			return fmt.Sprintf("❌ Invalid longitude in '%s'", arg), nil
		}
		
		waypoints = append(waypoints, spatial.Waypoint{
			Lat:  lat,
			Lon:  lon,
			Name: fmt.Sprintf("%.4f, %.4f", lat, lon),
		})
	}

	if len(waypoints) == 0 {
		return "❌ No valid waypoints provided", nil
	}

	// Queue the waypoints and start the first leg
	err := spatial.SetCourierWaypoints(waypoints)
	if err != nil {
		return fmt.Sprintf("❌ %v", err), nil
	}

	var names []string
	for _, wp := range waypoints {
		names = append(names, wp.Name)
	}

	return fmt.Sprintf("🚴 Route queued: %s", strings.Join(names, " → ")), nil
}
