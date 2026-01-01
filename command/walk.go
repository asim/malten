package command

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const osrmURL = "http://router.project-osrm.org/route/v1/foot"

func init() {
	Register(&Command{
		Name:        "walk",
		Description: "Walking time to a destination",
		Usage:       "/walk <destination>",
		Handler:     handleWalk,
		Match:       matchWalk,
	})
}

// matchWalk detects walking queries in natural language
// Handles: "how long to walk to X", "walk to X", "walking time to X", "how far is X"
func matchWalk(input string) (bool, []string) {
	lower := strings.ToLower(input)
	
	patterns := []string{
		"how long to walk to ",
		"how far to walk to ",
		"walking time to ",
		"walk time to ",
		"walk to ",
		"how far is ",
		"how long to ",
	}
	
	for _, p := range patterns {
		if idx := strings.Index(lower, p); idx != -1 {
			dest := strings.TrimSpace(input[idx+len(p):])
			dest = strings.TrimSuffix(dest, "?")
			if dest != "" {
				return true, []string{dest}
			}
		}
	}
	return false, nil
}

// handleWalk handles the /walk command
func handleWalk(ctx *Context, args []string) (string, error) {
	if len(args) == 0 {
		return "Usage: /walk <destination>\nExample: /walk Twickenham Station", nil
	}
	
	if !ctx.HasLocation() {
		return "", fmt.Errorf("enable location first to get walking time")
	}
	
	destination := strings.Join(args, " ")
	return WalkTo(ctx.Lat, ctx.Lon, destination)
}

// WalkTo calculates walking time from user's location to a destination
func WalkTo(fromLat, fromLon float64, destination string) (string, error) {
	// Geocode destination
	toLat, toLon, err := Geocode(destination)
	if err != nil {
		return "", fmt.Errorf("couldn't find %s", destination)
	}

	// Get walking route from OSRM
	url := fmt.Sprintf("%s/%f,%f;%f,%f?overview=false",
		osrmURL, fromLon, fromLat, toLon, toLat)

	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Malten/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("routing error")
	}
	defer resp.Body.Close()

	var data struct {
		Code   string `json:"code"`
		Routes []struct {
			Duration float64 `json:"duration"`
			Distance float64 `json:"distance"`
		} `json:"routes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", fmt.Errorf("routing error")
	}

	if data.Code != "Ok" || len(data.Routes) == 0 {
		return "", fmt.Errorf("no route found")
	}

	route := data.Routes[0]
	km := route.Distance / 1000
	
	// Calculate walking time at 5 km/h (OSRM public server doesn't have foot profile)
	minutes := int(km * 60 / 5)

	if route.Distance < 50 {
		return fmt.Sprintf("🚶 %s is right here (%.0fm)", destination, route.Distance), nil
	} else if minutes <= 1 {
		return fmt.Sprintf("🚶 %s · 1 min walk (%.0fm)", destination, route.Distance), nil
	}
	return fmt.Sprintf("🚶 %s · %d min walk (%.1f km)", destination, minutes, km), nil
}
