package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

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
