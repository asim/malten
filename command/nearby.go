package command

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"malten.ai/spatial"
)

const overpassURL = "https://overpass-api.de/api/interpreter"
const nominatimURL = "https://nominatim.openstreetmap.org/search"

// urlEncode encodes a string for URLs, including apostrophes
func urlEncode(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "'", "%27")
}

// Geocode converts a place name to coordinates using Nominatim
func Geocode(place string) (float64, float64, error) {
	return GeocodeNear(place, 0, 0)
}

// GeocodeNear geocodes with location bias - prefers results near the given point
func GeocodeNear(place string, nearLat, nearLon float64) (float64, float64, error) {
	// Build URL with query params
	req, _ := http.NewRequest("GET", nominatimURL, nil)
	q := req.URL.Query()
	q.Add("q", place)
	q.Add("format", "json")
	q.Add("limit", "5") // Get multiple results to pick closest

	// Add viewbox to bias towards user's location (roughly 50km box)
	if nearLat != 0 && nearLon != 0 {
		q.Add("viewbox", fmt.Sprintf("%.4f,%.4f,%.4f,%.4f",
			nearLon-0.5, nearLat+0.5, nearLon+0.5, nearLat-0.5))
		q.Add("bounded", "0") // Prefer but don't require results in viewbox
	}

	fullURL := nominatimURL + "?" + q.Encode()
	resp, err := spatial.LocationGet(fullURL)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()

	var results []struct {
		Lat string `json:"lat"`
		Lon string `json:"lon"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return 0, 0, err
	}

	if len(results) == 0 {
		return 0, 0, fmt.Errorf("not found")
	}

	// If we have a reference point, pick the closest result
	if nearLat != 0 && nearLon != 0 && len(results) > 1 {
		var bestLat, bestLon float64
		bestDist := 999999.0
		for _, r := range results {
			lat, _ := strconv.ParseFloat(r.Lat, 64)
			lon, _ := strconv.ParseFloat(r.Lon, 64)
			dist := (lat-nearLat)*(lat-nearLat) + (lon-nearLon)*(lon-nearLon)
			if dist < bestDist {
				bestDist = dist
				bestLat, bestLon = lat, lon
			}
		}
		return bestLat, bestLon, nil
	}

	lat, _ := strconv.ParseFloat(results[0].Lat, 64)
	lon, _ := strconv.ParseFloat(results[0].Lon, 64)
	return lat, lon, nil
}

// nearby is NOT registered as a pluggable command because it needs
// session token context from the server handler

// Location holds user's current location
type LocationPoint struct {
	Lat  float64
	Lon  float64
	Time time.Time
}

type Location struct {
	Lat           float64
	Lon           float64
	UpdatedAt     time.Time
	History       []LocationPoint // Last N location updates
	CheckedIn     *CheckIn        // Manual location override
	PromptedAt    time.Time       // Last time we prompted for check-in
	LastArrivalAt time.Time       // Last time we detected arrival (cooldown)
}

type CheckIn struct {
	Name string
	Lat  float64
	Lon  float64
	Time time.Time
}

const (
	locationHistorySize    = 20   // Keep 20 points for arrival detection (~60 seconds at 3s intervals)
	gpsStuckThreshold      = 30.0 // meters - if all points within this, GPS is "stuck"
	gpsStuckDuration       = 5 * time.Minute
	checkInPromptCooldown  = 10 * time.Minute
	checkInExpiry          = 2 * time.Hour
	arrivalSpeedThreshold  = 1.0  // m/s - below this is "stopped" (slow stroll)
	passingSpeedThreshold  = 2.0  // m/s - above this is "passing" (normal walk)
	arrivalMovingThreshold = 1.5  // m/s - above this means "was moving" (reduced from 5.0)
	arrivalPOIRadius       = 50.0 // meters - look for POIs within this distance (reduced from 100)
)

// Global location store (token -> location)
// Token is generated client-side and stored in sessionStorage
var (
	locations   = make(map[string]*Location)
	locationsMu sync.RWMutex
	locationTTL = 5 * time.Minute
)

func init() {
	// Register nearby command
	Register(&Command{
		Name:        "nearby",
		Description: "Find nearby places",
		Usage:       "/nearby <type> [location]",
		Emoji:       "🔍",
		LoadingText: "Searching for %s nearby...",
		Handler:     handleNearby,
		Match:       matchNearby,
	})

	// Register place info command (hours, etc)
	Register(&Command{
		Name:        "place",
		Description: "Get info about a specific place",
		Usage:       "/place <name>",
		Emoji:       "📍",
		LoadingText: "Looking up %s...",
		Handler:     handlePlaceInfo,
		Match:       matchPlaceInfo,
	})

	// Register check-in command
	Register(&Command{
		Name:        "checkin",
		Description: "Check in to a location",
		Usage:       "/checkin <place name>",
		Emoji:       "📍",
		LoadingText: "Checking in to %s...",
		Handler:     handleCheckIn,
	})

	Register(&Command{
		Name:        "checkout",
		Description: "Clear check-in",
		Usage:       "/checkout",
		Emoji:       "📍",
		Handler: func(ctx *Context, args []string) (string, error) {
			if checkIn := GetCheckIn(ctx.Session); checkIn != nil {
				name := checkIn.Name
				ClearCheckIn(ctx.Session)
				return "📍 Checked out from " + name, nil
			}
			return "📍 Not checked in anywhere", nil
		},
	})

	// Register ping command (location update + context)
	Register(&Command{
		Name:        "ping",
		Description: "Update location and get context",
		Usage:       "/ping (with lat/lon params)",
		Handler:     handlePing,
	})

	// Cleanup expired locations every minute
	go func() {
		for {
			time.Sleep(time.Minute)
			cleanupLocations()
		}
	}()
}

func cleanupLocations() {
	locationsMu.Lock()
	defer locationsMu.Unlock()

	now := time.Now()
	for token, loc := range locations {
		if now.Sub(loc.UpdatedAt) > locationTTL {
			delete(locations, token)
		}
	}
}

// LocationUpdate contains the result of a location update
type LocationUpdate struct {
	ShouldPromptCheckIn bool   // GPS appears stuck, prompt for check-in
	ArrivedAt           string // POI name if stopped near POI
	PassingBy           string // POI name if moving past POI
	IsHome              bool   // True if arrived at a saved place
}

// SetLocation stores location for a session token and updates their view
// Returns LocationUpdate with prompts/notifications
func SetLocation(token string, lat, lon float64) *LocationUpdate {
	log.Printf("[nearby] SetLocation(%s, %f, %f)", token, lat, lon)
	now := time.Now()
	result := &LocationUpdate{}

	locationsMu.Lock()
	loc := locations[token]
	if loc == nil {
		loc = &Location{}
		locations[token] = loc
	}

	// Add to history
	loc.History = append(loc.History, LocationPoint{Lat: lat, Lon: lon, Time: now})
	if len(loc.History) > locationHistorySize {
		loc.History = loc.History[1:]
	}

	// Check if user moved significantly from check-in location
	if loc.CheckedIn != nil {
		dist := haversineMeters(lat, lon, loc.CheckedIn.Lat, loc.CheckedIn.Lon)
		if dist > 200 || now.Sub(loc.CheckedIn.Time) > checkInExpiry {
			log.Printf("[nearby] Clearing check-in for %s (moved %.0fm or expired)", token, dist)
			loc.CheckedIn = nil
		}
	}

	loc.Lat = lat
	loc.Lon = lon
	loc.UpdatedAt = now

	// Take snapshot of history for detection (to release lock)
	historyCopy := make([]LocationPoint, len(loc.History))
	copy(historyCopy, loc.History)

	// Check if GPS is stuck and we should prompt
	if loc.CheckedIn == nil && isGpsStuck(loc.History) {
		if now.Sub(loc.PromptedAt) > checkInPromptCooldown {
			result.ShouldPromptCheckIn = true
			loc.PromptedAt = now
		}
	}
	locationsMu.Unlock()

	// Detect proximity to POI (arriving or passing)
	// Only if not already checked in and not recently prompted
	locationsMu.Lock()
	canPrompt := loc.CheckedIn == nil && time.Since(loc.LastArrivalAt) > checkInPromptCooldown
	locationsMu.Unlock()

	if !result.ShouldPromptCheckIn && canPrompt {
		proximity := detectProximity(historyCopy, lat, lon)
		if proximity != nil {
			if proximity.IsStopped {
				result.ArrivedAt = proximity.POIName
				locationsMu.Lock()
				loc.LastArrivalAt = time.Now()
				locationsMu.Unlock()
			} else {
				// Only show "passing" if moving at normal walking speed or faster
				if proximity.RecentSpeed >= passingSpeedThreshold {
					result.PassingBy = proximity.POIName
				}
			}
		}
	}

	// Insert/update user in spatial index
	updateUserInSpatialIndex(token, lat, lon)

	// Ensure agent exists for this area (creates and starts indexing if new)
	db := spatial.Get()
	db.FindOrCreateAgent(lat, lon)

	// Context is built on-demand in handlePing, not here
	// This avoids duplicate calls

	return result
}

// isGpsStuck checks if all recent locations are within threshold distance
func isGpsStuck(history []LocationPoint) bool {
	if len(history) < 3 {
		return false
	}

	// Need at least gpsStuckDuration of history
	oldest := history[0]
	newest := history[len(history)-1]
	if newest.Time.Sub(oldest.Time) < gpsStuckDuration {
		return false
	}

	// Check if all points are within threshold of first point
	for i := 1; i < len(history); i++ {
		dist := haversineMeters(oldest.Lat, oldest.Lon, history[i].Lat, history[i].Lon)
		if dist > gpsStuckThreshold {
			return false
		}
	}
	return true
}

// haversineMeters returns distance between two points in meters
func haversineMeters(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000 // Earth radius in meters
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return R * c
}

// ProximityInfo contains info about nearby POI detection
type ProximityInfo struct {
	POIName     string
	IsStopped   bool    // true = arrived, false = passing
	RecentSpeed float64 // for debugging
}

// detectProximity checks if user is near a POI and whether they've stopped or are passing
func detectProximity(history []LocationPoint, lat, lon float64) *ProximityInfo {
	log.Printf("[nearby] detectProximity called with %d history points", len(history))

	// Check for nearby POI first - if nothing nearby, skip the speed calc
	db := spatial.Get()
	pois := db.Query(lat, lon, arrivalPOIRadius, spatial.EntityPlace, 1)
	if len(pois) == 0 {
		return nil
	}

	poiName := pois[0].Name

	// Need some history to calculate speed
	if len(history) < 3 {
		// Not enough history - assume passing (don't know if stopped)
		return &ProximityInfo{POIName: poiName, IsStopped: false, RecentSpeed: -1}
	}

	// Calculate recent speed from last few points
	recentPoints := history
	if len(recentPoints) > 5 {
		recentPoints = history[len(history)-5:]
	}

	// Speed = distance covered / time
	first := recentPoints[0]
	last := recentPoints[len(recentPoints)-1]
	dist := haversineMeters(first.Lat, first.Lon, last.Lat, last.Lon)
	duration := last.Time.Sub(first.Time).Seconds()

	recentSpeed := 0.0
	if duration > 0 {
		recentSpeed = dist / duration
	}

	log.Printf("[nearby] Near %s: recentSpeed=%.2f m/s (stopped<%.1f, passing>%.1f)",
		poiName, recentSpeed, arrivalSpeedThreshold, passingSpeedThreshold)

	isStopped := recentSpeed < arrivalSpeedThreshold

	return &ProximityInfo{
		POIName:     poiName,
		IsStopped:   isStopped,
		RecentSpeed: recentSpeed,
	}
}

func updateUserInSpatialIndex(token string, lat, lon float64) {
	db := spatial.Get()
	expiry := time.Now().Add(locationTTL)

	user := &spatial.Entity{
		ID:        "user-" + token[:8],
		Type:      spatial.EntityPerson,
		Name:      "user",
		Lat:       lat,
		Lon:       lon,
		Data:      map[string]interface{}{"token": token},
		ExpiresAt: &expiry,
	}
	db.Insert(user)
}

// User context cache
var (
	userContexts   = make(map[string]string)
	userContextsMu sync.RWMutex
)

func updateUserContext(token string, lat, lon float64) {
	ctxData := spatial.GetContextData(lat, lon)

	userContextsMu.Lock()
	userContexts[token] = ctxData.HTML // Store HTML for AI context
	userContextsMu.Unlock()
}

// GetUserContext returns the pre-built context for a user
func GetUserContext(token string) string {
	userContextsMu.RLock()
	defer userContextsMu.RUnlock()
	return userContexts[token]
}

// GetLocation retrieves location for a session token
// GetSpeed returns current speed estimate in m/s based on recent location history
func GetSpeed(token string) float64 {
	locationsMu.RLock()
	defer locationsMu.RUnlock()
	loc := locations[token]
	if loc == nil || len(loc.History) < 2 {
		return 0
	}

	// Calculate speed from last 2 points
	n := len(loc.History)
	p1 := loc.History[n-2]
	p2 := loc.History[n-1]

	dist := haversineMeters(p1.Lat, p1.Lon, p2.Lat, p2.Lon)
	dt := p2.Time.Sub(p1.Time).Seconds()
	if dt <= 0 {
		return 0
	}
	return dist / dt
}

func GetLocation(token string) *Location {
	locationsMu.RLock()
	defer locationsMu.RUnlock()

	loc := locations[token]
	if loc == nil {
		return nil
	}

	// Check if expired
	if time.Since(loc.UpdatedAt) > locationTTL {
		return nil
	}
	return loc
}

// ClearLocation removes location for a session token
func ClearLocation(token string) {
	locationsMu.Lock()
	defer locationsMu.Unlock()
	delete(locations, token)
}

// SetCheckIn manually sets user's location to a named place
func SetCheckIn(token string, name string, lat, lon float64) {
	locationsMu.Lock()
	defer locationsMu.Unlock()

	loc := locations[token]
	if loc == nil {
		loc = &Location{}
		locations[token] = loc
	}

	loc.CheckedIn = &CheckIn{
		Name: name,
		Lat:  lat,
		Lon:  lon,
		Time: time.Now(),
	}
}

// GetCheckIn returns the current check-in for a user, if any
func GetCheckIn(token string) *CheckIn {
	locationsMu.RLock()
	defer locationsMu.RUnlock()

	loc := locations[token]
	if loc == nil || loc.CheckedIn == nil {
		return nil
	}

	// Check if expired
	if time.Since(loc.CheckedIn.Time) > checkInExpiry {
		return nil
	}
	return loc.CheckedIn
}

// ClearCheckIn removes the check-in for a user
func ClearCheckIn(token string) {
	locationsMu.Lock()
	defer locationsMu.Unlock()

	if loc := locations[token]; loc != nil {
		loc.CheckedIn = nil
	}
}

// OSM element from Overpass API
type OSMElement struct {
	Type   string            `json:"type"`
	ID     int64             `json:"id"`
	Lat    float64           `json:"lat"`
	Lon    float64           `json:"lon"`
	Center *OSMCenter        `json:"center,omitempty"`
	Tags   map[string]string `json:"tags"`
}

type OSMCenter struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// GetCoords returns lat/lon, using center for ways
func (e *OSMElement) GetCoords() (float64, float64) {
	if e.Lat != 0 || e.Lon != 0 {
		return e.Lat, e.Lon
	}
	if e.Center != nil {
		return e.Center.Lat, e.Center.Lon
	}
	return 0, 0
}

type OverpassResponse struct {
	Elements []OSMElement `json:"elements"`
}

// ValidTypes is the set of recognized place types
var ValidTypes = map[string]bool{
	"cafe": true, "cafes": true, "coffee": true,
	"restaurant": true, "restaurants": true, "food": true,
	"bar": true, "bars": true,
	"pub": true, "pubs": true,
	"pharmacy": true, "pharmacies": true,
	"hospital": true, "hospitals": true,
	"bank": true, "banks": true,
	"atm": true, "atms": true,
	"supermarket": true, "grocery": true,
	"shop": true, "shops": true, "store": true,
	"gas": true, "petrol": true, "fuel": true, "station": true,
	"parking": true,
	"gym":     true,
	"mosque":  true, "church": true, "temple": true,
	"hotel": true, "hotels": true,
	"cinema": true, "cinemas": true, "theatre": true, "theater": true,
}

// IsValidPlaceType checks if a string is a recognized place type
func IsValidPlaceType(s string) bool {
	return ValidTypes[s]
}

// MultiWordTypes maps multi-word phrases to canonical types
var MultiWordTypes = map[string]string{
	"petrol station": "fuel",
	"gas station":    "fuel",
	"fuel station":   "fuel",
	"coffee shop":    "cafe",
	"coffee shops":   "cafe",
}

// CheckMultiWordType checks if input contains a multi-word type, returns type and remaining words
func CheckMultiWordType(words []string) (string, []string) {
	input := strings.ToLower(strings.Join(words, " "))
	for phrase, placeType := range MultiWordTypes {
		if strings.Contains(input, phrase) {
			// Remove the phrase words from the list
			var remaining []string
			phraseWords := strings.Fields(phrase)
			skipNext := 0
			for i, w := range words {
				if skipNext > 0 {
					skipNext--
					continue
				}
				// Check if this starts the phrase
				if i+len(phraseWords) <= len(words) {
					match := true
					for j, pw := range phraseWords {
						if strings.ToLower(words[i+j]) != pw {
							match = false
							break
						}
					}
					if match {
						skipNext = len(phraseWords) - 1
						continue
					}
				}
				remaining = append(remaining, w)
			}
			return placeType, remaining
		}
	}
	return "", words
}

// placeTypes maps common terms to OSM amenity/shop types
var placeTypes = map[string]string{
	"cafe":           "amenity=cafe",
	"cafes":          "amenity=cafe",
	"coffee":         "amenity=cafe",
	"restaurant":     "amenity=restaurant",
	"restaurants":    "amenity=restaurant",
	"food":           "amenity=restaurant",
	"bar":            "amenity=bar",
	"bars":           "amenity=bar",
	"pub":            "amenity=pub",
	"pubs":           "amenity=pub",
	"pharmacy":       "amenity=pharmacy",
	"pharmacies":     "amenity=pharmacy",
	"hospital":       "amenity=hospital",
	"hospitals":      "amenity=hospital",
	"bank":           "amenity=bank",
	"banks":          "amenity=bank",
	"atm":            "amenity=atm",
	"atms":           "amenity=atm",
	"supermarket":    "shop=supermarket",
	"grocery":        "shop=supermarket",
	"shop":           "shop=convenience",
	"shops":          "shop=convenience",
	"store":          "shop=convenience",
	"gas":            "amenity=fuel",
	"petrol":         "amenity=fuel",
	"fuel":           "amenity=fuel",
	"petrol station": "amenity=fuel",
	"gas station":    "amenity=fuel",
	"parking":        "amenity=parking",
	"gym":            "leisure=fitness_centre",
	"mosque":         "amenity=place_of_worship][religion=muslim",
	"church":         "amenity=place_of_worship][religion=christian",
	"temple":         "amenity=place_of_worship",
	"hotel":          "tourism=hotel",
	"hotels":         "tourism=hotel",
	"cinema":         "amenity=cinema",
	"cinemas":        "amenity=cinema",
	"theatre":        "amenity=theatre",
	"theater":        "amenity=theatre",
}

const searchRadius = 2000.0 // 2km default radius

// getSearchRadius returns appropriate search radius for different place types
func getSearchRadius(placeType string) float64 {
	// Sparse POIs need larger radius
	sparseTypes := map[string]bool{
		"cinema": true, "cinemas": true,
		"theatre": true, "theater": true,
		"hospital": true, "hospitals": true,
		"hotel": true, "hotels": true,
		"gym": true,
	}
	if sparseTypes[placeType] {
		return 5000.0 // 5km for sparse POIs
	}
	return searchRadius
}

// searchByName finds places by name in the spatial DB
func searchByName(name string, lat, lon float64) (string, error) {
	db := spatial.Get()
	results := db.FindByName(lat, lon, searchRadius*2, name, 10) // wider search for name

	if len(results) == 0 {
		// Try Foursquare as fallback
		if webResults := spatial.WebSearchPlaces(name, lat, lon); len(webResults) > 0 {
			return formatCachedEntities(webResults, name+" (web)"), nil
		}
		return fmt.Sprintf("No '%s' found nearby. Try /nearby cafes to see what's around.", name), nil
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("📍 FOUND '%s'\n\n", strings.ToUpper(name)))

	for i, e := range results {
		if i >= 5 {
			break
		}
		// Google Maps link with name and coordinates
		mapLink := fmt.Sprintf("https://www.google.com/maps/search/%s/@%f,%f,17z", urlEncode(e.Name), e.Lat, e.Lon)
		result.WriteString(fmt.Sprintf("• %s · %s\n", e.Name, mapLink))

		// Add address
		var tags map[string]string
		if placeData := e.GetPlaceData(); placeData != nil {
			tags = placeData.Tags
		} else if m, ok := e.Data.(map[string]interface{}); ok {
			if tagsData, ok := m["tags"].(map[string]interface{}); ok {
				tags = make(map[string]string)
				for k, v := range tagsData {
					if s, ok := v.(string); ok {
						tags[k] = s
					}
				}
			}
		}
		if addr := formatAddress(tags); addr != "" {
			result.WriteString(fmt.Sprintf("  %s\n", addr))
		}
		result.WriteString("\n")
	}

	return strings.TrimSpace(result.String()), nil
}

// NearbyWithLocation performs the actual nearby search with coordinates
const agentRadius = 5000.0 // 5km agent territory

func NearbyWithLocation(placeType string, lat, lon float64) (string, error) {
	log.Printf("[nearby] NearbyWithLocation(%s, %f, %f)", placeType, lat, lon)
	placeType = strings.ToLower(strings.TrimSpace(placeType))

	// If not a valid type, search by name instead
	if !ValidTypes[placeType] {
		return searchByName(placeType, lat, lon)
	}

	category := normalizeType(placeType)

	// Find or create agent for this area
	db := spatial.Get()
	db.FindOrCreateAgent(lat, lon)

	// Check spatial DB first (use appropriate radius for place type)
	radius := getSearchRadius(placeType)
	cached := db.QueryPlaces(lat, lon, radius, category, 20)

	// For cinemas, merge supplementary data with cache
	if category == "cinema" {
		supplementary := spatial.GetSupplementaryCinemas(lat, lon, radius)
		if len(supplementary) > 0 {
			existing := make(map[string]bool)
			for _, e := range cached {
				existing[strings.ToLower(e.Name)] = true
			}
			for _, s := range supplementary {
				if !existing[strings.ToLower(s.Name)] {
					cached = append(cached, s)
				}
			}
		}
	}

	if len(cached) > 0 {
		return formatCachedEntities(cached, placeType), nil
	}

	// Not in cache, query OSM
	osmType, ok := placeTypes[placeType]
	if !ok {
		osmType = "amenity=" + placeType
	}

	query := fmt.Sprintf(`
[out:json][timeout:10];
(
  node[%s](around:%.0f,%f,%f);
  way[%s](around:%.0f,%f,%f);
);
out center 10;
`, osmType, radius, lat, lon, osmType, radius, lat, lon)

	log.Printf("[nearby] Querying OSM for %s around %.4f,%.4f (%.0fm)", osmType, lat, lon, radius)
	resp, err := spatial.OSMPost(overpassURL, query)
	if err != nil {
		log.Printf("[nearby] OSM query failed: %v", err)
		return "Error searching for places", err
	}
	defer resp.Body.Close()

	// Check for rate limiting or server errors
	if resp.StatusCode != 200 {
		log.Printf("[nearby] OSM returned status %d", resp.StatusCode)
		return fallbackGoogleMapsLink(placeType, lat, lon), nil
	}

	var data OverpassResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return fallbackGoogleMapsLink(placeType, lat, lon), nil
	}

	if len(data.Elements) == 0 {
		// Try web search as fallback before Google Maps link
		if webResults := spatial.WebSearchPlaces(placeType, lat, lon); len(webResults) > 0 {
			return formatCachedEntities(webResults, placeType+" (web)"), nil
		}
		return fallbackGoogleMapsLink(placeType, lat, lon), nil
	}

	// Cache the results in background
	go cacheOSMResults(data.Elements, category)

	// For cinemas, merge with supplementary data before formatting
	if category == "cinema" {
		supplementary := spatial.GetSupplementaryCinemas(lat, lon, radius)
		if len(supplementary) > 0 {
			// Convert OSM elements to entities for merge
			var allCinemas []*spatial.Entity
			existing := make(map[string]bool)
			for _, el := range data.Elements {
				eLat, eLon := el.GetCoords()
				e := &spatial.Entity{
					Type: spatial.EntityPlace,
					Name: el.Tags["name"],
					Lat:  eLat,
					Lon:  eLon,
					Data: map[string]interface{}{"tags": el.Tags},
				}
				allCinemas = append(allCinemas, e)
				existing[strings.ToLower(el.Tags["name"])] = true
			}
			for _, s := range supplementary {
				if !existing[strings.ToLower(s.Name)] {
					allCinemas = append(allCinemas, s)
				}
			}
			return formatCachedEntities(allCinemas, placeType), nil
		}
	}

	return formatOSMResults(data.Elements, placeType, lat, lon), nil
}

// normalizeType converts plural/alias types to singular canonical form
func normalizeType(placeType string) string {
	aliases := map[string]string{
		"cafes": "cafe", "coffee": "cafe",
		"restaurants": "restaurant", "food": "restaurant",
		"bars":       "bar",
		"pubs":       "pub",
		"pharmacies": "pharmacy",
		"hospitals":  "hospital",
		"banks":      "bank",
		"atms":       "atm",
		"grocery":    "supermarket",
		"shops":      "shop", "store": "shop",
		"petrol": "fuel", "gas": "fuel", "station": "fuel",
		"hotels":  "hotel",
		"cinemas": "cinema", "theatres": "theatre", "theaters": "theatre",
	}
	if canonical, ok := aliases[placeType]; ok {
		return canonical
	}
	return placeType
}

// cacheOSMResults stores OSM results as entities in the spatial DB
func cacheOSMResults(elements []OSMElement, category string) {
	db := spatial.Get()

	for _, el := range elements {
		lat, lon := el.GetCoords()
		if lat == 0 && lon == 0 {
			continue
		}

		// Convert tags to interface map
		tags := make(map[string]interface{})
		for k, v := range el.Tags {
			tags[k] = v
		}

		entity := &spatial.Entity{
			Type: spatial.EntityPlace,
			Name: el.Tags["name"],
			Lat:  lat,
			Lon:  lon,
			Data: map[string]interface{}{
				"category": category,
				"tags":     tags,
				"osm_id":   el.ID,
				"osm_type": el.Type,
			},
		}

		db.Insert(entity)
	}
}

// formatCachedEntities formats cached entities for display
func formatCachedEntities(entities []*spatial.Entity, placeType string) string {
	var result strings.Builder
	result.WriteString(fmt.Sprintf("📍 NEARBY %s (cached)\n\n", strings.ToUpper(placeType)))

	max := 8
	if len(entities) < max {
		max = len(entities)
	}

	for i := 0; i < max; i++ {
		e := entities[i]
		name := e.Name
		if name == "" {
			name = "(unnamed)"
		}

		// Extract website/phone from typed data or legacy
		var website, phone string
		var tags map[string]string
		if placeData := e.GetPlaceData(); placeData != nil {
			tags = placeData.Tags
		} else if m, ok := e.Data.(map[string]interface{}); ok {
			website, _ = m["website"].(string)
			phone, _ = m["phone"].(string)
			if tagsData, ok := m["tags"].(map[string]interface{}); ok {
				tags = make(map[string]string)
				for k, v := range tagsData {
					if s, ok := v.(string); ok {
						tags[k] = s
					}
				}
			}
		}

		// Try to get website/phone from tags if not set directly
		if len(tags) > 0 {
			if website == "" {
				website = tags["website"]
				if website == "" {
					website = tags["contact:website"]
				}
			}
			if phone == "" {
				phone = tags["phone"]
				if phone == "" {
					phone = tags["contact:phone"]
				}
			}
		}

		// Links: Map and Website (if available)
		mapLink := fmt.Sprintf("https://www.google.com/maps/search/%s/@%f,%f,17z", urlEncode(name), e.Lat, e.Lon)
		if website != "" {
			result.WriteString(fmt.Sprintf("• %s · %s · %s\n", name, mapLink, website))
		} else {
			result.WriteString(fmt.Sprintf("• %s · %s\n", name, mapLink))
		}

		// Address - check legacy data first, then tags
		var address string
		if m, ok := e.Data.(map[string]interface{}); ok {
			address, _ = m["address"].(string)
		}
		if address != "" {
			result.WriteString(fmt.Sprintf("  %s\n", address))
		} else if len(tags) > 0 {
			if addr := formatAddress(tags); addr != "" {
				result.WriteString(fmt.Sprintf("  %s\n", addr))
			}
		}

		// Hours
		if tags != nil {
			if hours := tags["opening_hours"]; hours != "" {
				result.WriteString(fmt.Sprintf("  🕐 %s\n", hours))
			}
		}

		// Phone
		if phone != "" {
			result.WriteString(fmt.Sprintf("  📞 %s\n", phone))
		}
		result.WriteString("\n")
	}

	if len(entities) > max {
		result.WriteString(fmt.Sprintf("...and %d more\n", len(entities)-max))
	}

	return strings.TrimSpace(result.String())
}

// formatOSMResults formats OSM results for display
func formatOSMResults(elements []OSMElement, placeType string, lat, lon float64) string {
	var result strings.Builder
	result.WriteString(fmt.Sprintf("📍 NEARBY %s\n\n", strings.ToUpper(placeType)))

	max := 8
	if len(elements) < max {
		max = len(elements)
	}

	for i := 0; i < max; i++ {
		el := elements[i]
		name := el.Tags["name"]
		if name == "" {
			name = "(unnamed)"
		}

		elLat, elLon := el.GetCoords()
		if elLat == 0 && elLon == 0 {
			elLat, elLon = lat, lon
		}

		// Google Maps link with name and coordinates
		mapLink := fmt.Sprintf("https://www.google.com/maps/search/%s/@%f,%f,17z", urlEncode(name), elLat, elLon)
		result.WriteString(fmt.Sprintf("• %s · %s\n", name, mapLink))

		if addr := formatAddress(el.Tags); addr != "" {
			result.WriteString(fmt.Sprintf("  %s\n", addr))
		}

		if hours := el.Tags["opening_hours"]; hours != "" {
			result.WriteString(fmt.Sprintf("  🕐 %s\n", hours))
		}
		result.WriteString("\n")
	}

	if len(elements) > max {
		result.WriteString(fmt.Sprintf("...and %d more\n", len(elements)-max))
	}

	return strings.TrimSpace(result.String())
}

// fallbackGoogleMapsLink returns a Google Maps search link when OSM fails
// ReverseGeocode gets area name from coordinates

func fallbackGoogleMapsLink(placeType string, lat, lon float64) string {
	searchTerm := urlEncode(placeType)
	link := fmt.Sprintf("https://www.google.com/maps/search/%s/@%f,%f,15z", searchTerm, lat, lon)
	return fmt.Sprintf("📍 Search %s on Google Maps:\n%s", placeType, link)
}

func formatAddress(tags map[string]string) string {
	var parts []string

	if street := tags["addr:street"]; street != "" {
		if num := tags["addr:housenumber"]; num != "" {
			parts = append(parts, num+" "+street)
		} else {
			parts = append(parts, street)
		}
	}

	if city := tags["addr:city"]; city != "" {
		parts = append(parts, city)
	}

	if postcode := tags["addr:postcode"]; postcode != "" {
		parts = append(parts, postcode)
	}

	return strings.Join(parts, ", ")
}

// matchNearby detects nearby queries in natural language
func matchNearby(input string) (bool, []string) {
	input = strings.TrimSpace(input)
	lower := strings.ToLower(input)
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return false, nil
	}

	// Don't match question patterns - those go to AI or place command
	questionPhrases := []string{"what time", "when does", "is .* open", "opening hours", "hours for", "what about", "tell me about", "info on", "information about", "how do i", "how to", "get to", "directions to", "way to"}
	for _, q := range questionPhrases {
		if strings.Contains(lower, q) || matchWildcard(lower, q) {
			return false, nil
		}
	}

	// Don't match "where is" questions - those go to AI with context
	if strings.HasPrefix(lower, "where is") || strings.HasPrefix(lower, "where's") {
		return false, nil
	}

	// Remove filler words
	var cleaned []string
	for _, p := range parts {
		l := strings.ToLower(p)
		if l == "near" || l == "nearby" || l == "me" || l == "in" || l == "around" ||
			l == "nearest" || l == "closest" || l == "find" || l == "show" || l == "where" ||
			l == "is" || l == "the" || l == "a" || l == "an" || l == "to" ||
			l == "what" || l == "about" || l == "any" || l == "are" || l == "there" {
			continue
		}
		cleaned = append(cleaned, p)
	}

	// Check for multi-word types first (e.g., "petrol station")
	multiType, remaining := CheckMultiWordType(cleaned)
	if multiType != "" {
		result := append([]string{multiType}, remaining...)
		return true, result
	}

	// Check if any word is a valid place type
	hasPlaceType := false
	for _, p := range cleaned {
		if IsValidPlaceType(strings.ToLower(p)) {
			hasPlaceType = true
			break
		}
	}

	if !hasPlaceType {
		return false, nil
	}

	// Only match explicit nearby queries - not just any sentence with a place type
	// "cafes near me" = yes, "I want coffee" = no (let AI handle it)
	nearbyPatterns := []string{"near me", "around me", "nearby", "closest", "nearest", "find a", "find me", "show me", "where can i find"}
	isNearbyQuery := false
	for _, p := range nearbyPatterns {
		if strings.Contains(lower, p) {
			isNearbyQuery = true
			break
		}
	}
	// Also match if it starts with a place type ("cafes?", "restaurants")
	if !isNearbyQuery && len(cleaned) == 1 && IsValidPlaceType(strings.ToLower(cleaned[0])) {
		isNearbyQuery = true
	}

	if isNearbyQuery {
		return true, cleaned
	}

	return false, nil
}

// handleNearby processes the nearby command
func handleNearby(ctx *Context, args []string) (string, error) {
	if len(args) == 0 {
		return "Usage: /nearby <type> [location]\nExamples: /nearby cafes, /nearby Twickenham cafes", nil
	}

	// Find the place type and location from args
	var placeType string
	var locationParts []string
	var lat, lon float64

	for _, arg := range args {
		lower := strings.ToLower(arg)
		if IsValidPlaceType(lower) {
			placeType = lower
		} else if coords := strings.Split(arg, ","); len(coords) == 2 {
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

	// If no valid place type found, use full input as search term (not as location!)
	if placeType == "" {
		placeType = strings.Join(args, " ")
		locationParts = nil // Don't geocode the search term itself
	}

	// Geocode location if provided (only if we have both a place type AND location parts)
	if lat == 0 && lon == 0 && len(locationParts) > 0 {
		placeName := strings.Join(locationParts, " ")
		geoLat, geoLon, err := Geocode(placeName)
		if err != nil {
			return "", fmt.Errorf("could not find location: %s", placeName)
		}
		lat, lon = geoLat, geoLon
	}

	// Fall back to user's location
	if lat == 0 && lon == 0 {
		if !ctx.HasLocation() {
			return "", fmt.Errorf("location not available. Enable location or specify: /nearby Twickenham cafes")
		}
		lat, lon = ctx.Lat, ctx.Lon
	}

	return NearbyWithLocation(placeType, lat, lon)
}

// matchPlaceInfo matches questions about specific places
// "What time does Sainsbury's close", "Is Boots open", "Sainsbury's hours"
func matchPlaceInfo(input string) (bool, []string) {
	lower := strings.ToLower(input)

	// Patterns that indicate a place-specific question
	patterns := []string{
		"what time does",
		"when does",
		"is .* open",
		"is .* closed",
		"hours for",
		"opening hours",
		"closing time",
	}

	for _, p := range patterns {
		if strings.Contains(lower, p) || matchWildcard(lower, p) {
			// Extract the place name
			name := extractPlaceName(input)
			if name != "" {
				return true, []string{name}
			}
		}
	}

	// Also match just "X hours" or "X closing"
	if strings.HasSuffix(lower, " hours") || strings.HasSuffix(lower, " closing") || strings.HasSuffix(lower, " open") {
		name := extractPlaceName(input)
		if name != "" {
			return true, []string{name}
		}
	}

	return false, nil
}

func matchWildcard(s, pattern string) bool {
	if !strings.Contains(pattern, ".*") {
		return strings.Contains(s, pattern)
	}
	parts := strings.Split(pattern, ".*")
	idx := 0
	for _, part := range parts {
		if part == "" {
			continue
		}
		found := strings.Index(s[idx:], part)
		if found == -1 {
			return false
		}
		idx += found + len(part)
	}
	return true
}

func getKeys(m map[string]interface{}) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func extractPlaceName(input string) string {
	lower := strings.ToLower(input)

	// Remove common question phrases first (order matters - longer first)
	phrases := []string{
		"what time does", "when does", "opening hours", "hours for",
		"closing time", "is the", "is a",
	}
	for _, p := range phrases {
		lower = strings.ReplaceAll(lower, p, " ")
	}

	// Remove individual words (as whole words only)
	words := strings.Fields(lower)
	removeWords := map[string]bool{
		"open": true, "closed": true, "close": true, "closing": true,
		"hours": true, "the": true, "a": true, "today": true, "now": true,
		"is": true, "are": true, "?": true,
	}

	var kept []string
	for _, w := range words {
		w = strings.Trim(w, "?")
		if !removeWords[w] && w != "" {
			kept = append(kept, w)
		}
	}

	return strings.Join(kept, " ")
}

func handlePlaceInfo(ctx *Context, args []string) (string, error) {
	if len(args) == 0 {
		return "Which place?", nil
	}

	placeName := strings.Join(args, " ")
	log.Printf("[place] Looking for %q at %.4f,%.4f", placeName, ctx.Lat, ctx.Lon)

	if !ctx.HasLocation() {
		return "Enable location to find places near you", nil
	}

	// Search spatial index for this place
	db := spatial.Get()
	places := db.Query(ctx.Lat, ctx.Lon, 5000, spatial.EntityPlace, 100) // 5km radius
	log.Printf("[place] Found %d places in 2km radius", len(places))

	// Find matching place
	var match *spatial.Entity
	for _, p := range places {
		lowerName := strings.ToLower(p.Name)
		if strings.Contains(lowerName, placeName) {
			match = p
			break
		}
	}

	if match == nil {
		// Log first few names to debug
		for i, p := range places {
			if i < 5 {
				log.Printf("[place] Place %d: %q", i, p.Name)
			}
		}
		return fmt.Sprintf("No %s found nearby", placeName), nil
	}
	log.Printf("[place] Found match: %s", match.Name)

	// Build response with hours
	var result strings.Builder
	result.WriteString(fmt.Sprintf("📍 %s\n", match.Name))

	// Extract place details from typed data or legacy
	var hours, phone, addr string
	var tags map[string]string
	if placeData := match.GetPlaceData(); placeData != nil {
		tags = placeData.Tags
	} else if m, ok := match.Data.(map[string]interface{}); ok {
		// Try direct fields first (legacy)
		hours, _ = m["opening_hours"].(string)
		phone, _ = m["phone"].(string)
		addr, _ = m["address"].(string)

		// Try tags (OSM data structure)
		if tagsRaw, ok := m["tags"].(map[string]interface{}); ok {
			tags = make(map[string]string)
			for k, v := range tagsRaw {
				if s, ok := v.(string); ok {
					tags[k] = s
				}
			}
		}
	}

	// Fill in from tags if available
	if len(tags) > 0 {
		if hours == "" {
			hours = tags["opening_hours"]
		}
		if phone == "" {
			phone = tags["phone"]
		}
		if addr == "" {
			// Build address from components
			var parts []string
			if num := tags["addr:housenumber"]; num != "" {
				parts = append(parts, num)
			}
			if street := tags["addr:street"]; street != "" {
				parts = append(parts, street)
			}
			if postcode := tags["addr:postcode"]; postcode != "" {
				parts = append(parts, postcode)
			}
			if len(parts) > 0 {
				addr = strings.Join(parts, " ")
			}
		}
	}

	if addr != "" {
		result.WriteString(fmt.Sprintf("%s\n", addr))
	}
	if hours != "" {
		result.WriteString(fmt.Sprintf("🕐 %s\n", hours))
	} else {
		result.WriteString("Hours not available\n")
	}
	if phone != "" {
		result.WriteString(fmt.Sprintf("📞 %s\n", phone))
	}

	// Add map link
	result.WriteString(fmt.Sprintf("https://maps.google.com/maps/search/%s/@%.6f,%.6f,17z",
		strings.ReplaceAll(match.Name, " ", "+"), match.Lat, match.Lon))

	return result.String(), nil
}

// handleCheckIn processes check-in to a named location
func handleCheckIn(ctx *Context, args []string) (string, error) {
	if len(args) == 0 {
		return "Usage: /checkin <place name>", nil
	}

	placeName := strings.Join(args, " ")

	if !ctx.HasLocation() {
		return "Enable location first", nil
	}

	// Search nearby POIs for matching name
	db := spatial.Get()
	pois := db.Query(ctx.Lat, ctx.Lon, 500, spatial.EntityPlace, 50)

	var match *spatial.Entity
	lowerName := strings.ToLower(placeName)
	for _, p := range pois {
		if strings.Contains(strings.ToLower(p.Name), lowerName) {
			match = p
			break
		}
	}

	if match == nil {
		return fmt.Sprintf("No place matching %q found nearby", placeName), nil
	}

	// Set the check-in
	SetCheckIn(ctx.Session, match.Name, match.Lat, match.Lon)
	log.Printf("[checkin] %s checked in to %s (%.4f, %.4f)", ctx.Session, match.Name, match.Lat, match.Lon)

	return fmt.Sprintf("📍 Checked in to %s", match.Name), nil
}

// handlePing processes location update and returns context as JSON
func handlePing(ctx *Context, args []string) (string, error) {
	if !ctx.HasLocation() {
		return "📍 Location not provided", nil
	}

	// Note: SetLocation already called by handler.go for all commands with lat/lon

	// Record location for route tracking (captures user's actual paths)
	spatial.RecordLocation(ctx.Session, ctx.Lat, ctx.Lon)

	// Update push manager with current location (for background notifications)
	UpdatePushLocation(ctx.Session, ctx.Lat, ctx.Lon)

	// Use check-in location if set, otherwise GPS
	contextLat, contextLon := ctx.Lat, ctx.Lon
	if checkIn := GetCheckIn(ctx.Session); checkIn != nil {
		contextLat, contextLon = checkIn.Lat, checkIn.Lon
	}

	// Get speed for change detection (suppress street changes while driving with poor GPS)
	speed := GetSpeed(ctx.Session)
	accuracy := ctx.Accuracy
	if accuracy == 0 {
		accuracy = 100 // Assume poor accuracy if not provided
	}

	// Get context with change detection
	contextData, changes := spatial.GetContextWithChanges(ctx.Session, contextLat, contextLon, accuracy, speed)

	// Cache for AI requests
	userContextsMu.Lock()
	userContexts[ctx.Session] = contextData.HTML
	userContextsMu.Unlock()

	// Push meaningful changes via websocket (handled by server)
	if len(changes) > 0 {
		ctx.PushMessages = changes
	}

	// Return JSON context
	b, _ := json.Marshal(contextData)
	return string(b), nil
}

// UpdatePushLocation is called by handlePing to update push manager
// Defined as variable so server package can set it (avoids import cycle)
var UpdatePushLocation = func(sessionID string, lat, lon float64) {}
