package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

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

	// Set cookie - session only (no expiry), httpOnly
	// Check X-Forwarded-Proto for HTTPS behind proxy, or localhost
	isSecure := r.Header.Get("X-Forwarded-Proto") == "https" || r.TLS != nil
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
	})

	return token
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

// haversine calculates distance between two points in km
func haversine(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371 // Earth's radius in km
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return R * c
}

// sendCheckInPrompt queries nearby POIs and sends a check-in prompt to the user
func sendCheckInPrompt(token, stream string, lat, lon float64) {
	db := spatial.Get()
	
	// Query nearby POIs (200m radius)
	pois := db.Query(lat, lon, 200, spatial.EntityPlace, 5)
	if len(pois) == 0 {
		return
	}
	
	// Build check-in prompt message
	var lines []string
	lines = append(lines, "📍 Where are you?")
	lines = append(lines, "")
	
	for _, poi := range pois {
		dist := haversine(lat, lon, poi.Lat, poi.Lon) * 1000 // km to m
		lines = append(lines, fmt.Sprintf("• %s (%.0fm)", poi.Name, dist))
	}
	
	lines = append(lines, "")
	lines = append(lines, "Reply with the name to check in, or ignore.")
	
	msg := &Message{
		Id:      Random(16),
		Type:    "message",
		Text:    strings.Join(lines, "\n"),
		Stream:  stream,
		Channel: "@" + token, // Private to this user
		Created: time.Now().UnixNano(),
	}
	
	Default.Broadcast(msg)
}
