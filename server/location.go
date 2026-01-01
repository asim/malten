package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/asim/malten/command"
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

	// Store location (expires after 5 min)
	command.SetLocation(token, lat, lon)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok": true,
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
// Supports: /nearby cafes, /nearby cafes Twickenham, /nearby cafes 51.4,-0.3
func HandleNearbyCommand(args []string, token string) string {
	if len(args) == 0 {
		return "Usage: /nearby <type> [location]\nExamples: /nearby cafes, /nearby cafes Twickenham, /nearby cafes 51.4,-0.3"
	}

	placeType := args[0]
	var lat, lon float64

	if len(args) >= 2 {
		// Check if second arg is coordinates (lat,lon)
		if coords := strings.Split(args[1], ","); len(coords) == 2 {
			if parsedLat, err := strconv.ParseFloat(coords[0], 64); err == nil {
				if parsedLon, err := strconv.ParseFloat(coords[1], 64); err == nil {
					lat, lon = parsedLat, parsedLon
				}
			}
		}
		// If not coords, it's a place name - geocode it
		if lat == 0 && lon == 0 {
			placeName := strings.Join(args[1:], " ")
			geoLat, geoLon, err := command.Geocode(placeName)
			if err != nil {
				return fmt.Sprintf("📍 Could not find location: %s", placeName)
			}
			lat, lon = geoLat, geoLon
		}
	} else {
		// No location specified, use user's ping location
		loc := command.GetLocation(token)
		if loc == nil {
			return "📍 Location not available. Enable location? Use /ping on\nOr specify: /nearby cafes Twickenham"
		}
		lat, lon = loc.Lat, loc.Lon
	}

	result, err := command.NearbyWithLocation(placeType, lat, lon)
	if err != nil {
		return "Error searching: " + err.Error()
	}
	return result
}
