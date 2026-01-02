package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"malten.ai/command"
	"malten.ai/spatial"
)

const sessionCookieName = "malten_session"

// getSessionToken retrieves or creates a session token from cookie
func getSessionToken(w http.ResponseWriter, r *http.Request) string {
	// Check for existing cookie
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && cookie.Value != "" {
		return cookie.Value
	}

	// Generate new token
	b := make([]byte, 16)
	rand.Read(b)
	token := hex.EncodeToString(b)

	// Set cookie - session only (no expiry), secure, httpOnly
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
	})

	return token
}

// ContextHandler returns local context for the user's session
func ContextHandler(w http.ResponseWriter, r *http.Request) {
	token := getSessionToken(w, r)
	
	// First check if we have pre-built context for this user
	if ctx := command.GetUserContext(token); ctx != "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"context": ctx})
		return
	}
	
	// Fall back to location from request params
	r.ParseForm()
	latStr := r.Form.Get("lat")
	lonStr := r.Form.Get("lon")

	if latStr == "" || lonStr == "" {
		// Try to get from stored location
		if loc := command.GetLocation(token); loc != nil {
			w.Header().Set("Content-Type", "application/json")
			ctx := spatial.GetLiveContext(loc.Lat, loc.Lon)
			json.NewEncoder(w).Encode(map[string]interface{}{"context": ctx})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"context": ""})
		return
	}

	lat, _ := strconv.ParseFloat(latStr, 64)
	lon, _ := strconv.ParseFloat(lonStr, 64)

	context := spatial.GetLiveContext(lat, lon)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"context": context})
}

// PingHandler receives location updates from clients
func PingHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	token := getSessionToken(w, r)
	r.ParseForm()

	latStr := r.Form.Get("lat")
	lonStr := r.Form.Get("lon")

	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		http.Error(w, "Invalid latitude", 400)
		return
	}

	lon, err := strconv.ParseFloat(lonStr, 64)
	if err != nil {
		http.Error(w, "Invalid longitude", 400)
		return
	}

	// Store location and update user in quadtree
	// This also builds their context view in background
	command.SetLocation(token, lat, lon)

	// Return context immediately from spatial index
	context := spatial.GetLiveContext(lat, lon)
	stream := spatial.StreamFromLocation(lat, lon)
	news := spatial.GetBreakingNews()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":      true,
		"context": context,
		"stream":  stream,
		"news":    news,
	})
}

// HandlePingCommand handles /ping on/off commands
func HandlePingCommand(cmd string, token string) string {
	parts := strings.Fields(cmd)
	if len(parts) < 2 {
		loc := command.GetLocation(token)
		if loc != nil {
			return fmt.Sprintf("📍 Location enabled (%.4f, %.4f)\nExpires in 5 min. Use /ping off to disable.", loc.Lat, loc.Lon)
		}
		return "📍 Location is disabled\nUse /ping on to enable (expires after 5 min)"
	}

	action := strings.ToLower(parts[1])
	switch action {
	case "on":
		// The actual location will be sent via /ping from the client
		return "📍 Requesting location... Please allow location access in your browser."
	case "off":
		command.ClearLocation(token)
		return "📍 Location sharing disabled"
	default:
		return "Usage: /ping on|off"
	}
}

// HandleNearbyCommand processes nearby requests with location
// Supports: /nearby cafes, /nearby Twickenham cafes, /nearby petrol station, /nearby cafes 51.4,-0.3
func HandleNearbyCommand(args []string, token string) string {
	if len(args) == 0 {
		return "Usage: /nearby <type> [location]\nExamples: /nearby cafes, /nearby Twickenham cafes, /nearby petrol station"
	}

	// Find the place type and location from args
	// Place type can be anywhere, location is everything else
	var placeType string
	var locationParts []string
	var lat, lon float64

	for _, arg := range args {
		lower := strings.ToLower(arg)
		// Check if this arg is a known place type
		if command.IsValidPlaceType(lower) {
			placeType = lower
		} else if coords := strings.Split(arg, ","); len(coords) == 2 {
			// Check if it's coordinates
			if parsedLat, err := strconv.ParseFloat(coords[0], 64); err == nil {
				if parsedLon, err := strconv.ParseFloat(coords[1], 64); err == nil {
					lat, lon = parsedLat, parsedLon
					continue
				}
			}
			locationParts = append(locationParts, arg)
		} else {
			locationParts = append(locationParts, arg)
		}
	}

	// If no place type found, use first arg as search term
	if placeType == "" {
		placeType = strings.Join(args, " ")
	}

	// If we have location parts but no coords, geocode them
	if lat == 0 && lon == 0 && len(locationParts) > 0 {
		placeName := strings.Join(locationParts, " ")
		geoLat, geoLon, err := command.Geocode(placeName)
		if err != nil {
			return fmt.Sprintf("📍 Could not find location: %s", placeName)
		}
		lat, lon = geoLat, geoLon
	}

	// If still no location, use user's ping location
	if lat == 0 && lon == 0 {
		loc := command.GetLocation(token)
		if loc == nil {
			return "📍 Location not available. Enable location? Use /ping on\nOr specify: /nearby Twickenham cafes"
		}
		lat, lon = loc.Lat, loc.Lon
	}

	result, err := command.NearbyWithLocation(placeType, lat, lon)
	if err != nil {
		return "Error searching: " + err.Error()
	}
	return result
}

// HandleAgentsCommand lists spatial agents
func HandleAgentsCommand() string {
	db := spatial.Get()
	agents := db.ListAgents()

	if len(agents) == 0 {
		return "🤖 No agents.\nAgents are created when you search a new area."
	}

	var result strings.Builder
	result.WriteString("🤖 AGENTS\n\n")

	for _, a := range agents {
		radius, _ := a.Data["radius"].(float64)
		status, _ := a.Data["status"].(string)
		poiCount, _ := a.Data["poi_count"].(float64)
		lastIndex, _ := a.Data["last_index"].(string)

		result.WriteString(fmt.Sprintf("• %s\n", a.Name))
		result.WriteString(fmt.Sprintf("  📍 %.4f, %.4f (%.0fm)\n", a.Lat, a.Lon, radius))
		result.WriteString(fmt.Sprintf("  📊 %d POIs | %s\n", int(poiCount), status))
		if lastIndex != "" {
			result.WriteString(fmt.Sprintf("  🕐 %s\n", lastIndex))
		}
		result.WriteString("\n")
	}

	return strings.TrimSpace(result.String())
}
