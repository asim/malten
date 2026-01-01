package command

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const overpassURL = "https://overpass-api.de/api/interpreter"
const nominatimURL = "https://nominatim.openstreetmap.org/search"

// Geocode converts a place name to coordinates using Nominatim
func Geocode(place string) (float64, float64, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", nominatimURL, nil)
	q := req.URL.Query()
	q.Add("q", place)
	q.Add("format", "json")
	q.Add("limit", "1")
	req.URL.RawQuery = q.Encode()
	req.Header.Set("User-Agent", "Malten/1.0")

	resp, err := client.Do(req)
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

	lat, _ := strconv.ParseFloat(results[0].Lat, 64)
	lon, _ := strconv.ParseFloat(results[0].Lon, 64)
	return lat, lon, nil
}

// nearby is NOT registered as a pluggable command because it needs
// session token context from the server handler

// Location holds user's current location
type Location struct {
	Lat       float64
	Lon       float64
	UpdatedAt time.Time
}

// Global location store (token -> location)
// Token is generated client-side and stored in sessionStorage
var (
	locations   = make(map[string]*Location)
	locationsMu sync.RWMutex
	locationTTL = 5 * time.Minute
)

func init() {
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

// SetLocation stores location for a session token
func SetLocation(token string, lat, lon float64) {
	locationsMu.Lock()
	defer locationsMu.Unlock()
	
	locations[token] = &Location{
		Lat:       lat,
		Lon:       lon,
		UpdatedAt: time.Now(),
	}
}

// GetLocation retrieves location for a session token
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

// validTypes is the set of recognized place types
var validTypes = map[string]bool{
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
	"gas": true, "petrol": true, "fuel": true,
	"parking": true,
	"gym": true,
	"mosque": true, "church": true, "temple": true,
	"hotel": true, "hotels": true,
}

// placeTypes maps common terms to OSM amenity/shop types
var placeTypes = map[string]string{
	"cafe":        "amenity=cafe",
	"cafes":       "amenity=cafe",
	"coffee":      "amenity=cafe",
	"restaurant":  "amenity=restaurant",
	"restaurants": "amenity=restaurant",
	"food":        "amenity=restaurant",
	"bar":         "amenity=bar",
	"bars":        "amenity=bar",
	"pub":         "amenity=pub",
	"pubs":        "amenity=pub",
	"pharmacy":    "amenity=pharmacy",
	"pharmacies":  "amenity=pharmacy",
	"hospital":    "amenity=hospital",
	"hospitals":   "amenity=hospital",
	"bank":        "amenity=bank",
	"banks":       "amenity=bank",
	"atm":         "amenity=atm",
	"atms":        "amenity=atm",
	"supermarket": "shop=supermarket",
	"grocery":     "shop=supermarket",
	"shop":        "shop=convenience",
	"shops":       "shop=convenience",
	"store":       "shop=convenience",
	"gas":         "amenity=fuel",
	"petrol":      "amenity=fuel",
	"fuel":        "amenity=fuel",
	"parking":     "amenity=parking",
	"gym":         "leisure=fitness_centre",
	"mosque":      "amenity=place_of_worship][religion=muslim",
	"church":      "amenity=place_of_worship][religion=christian",
	"temple":      "amenity=place_of_worship",
	"hotel":       "tourism=hotel",
	"hotels":      "tourism=hotel",
}



const searchRadius = 1000.0 // 1km radius

// searchByName finds places by name in the spatial DB
func searchByName(name string, lat, lon float64) (string, error) {
	db := GetSpatialDB()
	results := db.FindByName(lat, lon, searchRadius*2, name, 10) // wider search for name

	if len(results) == 0 {
		return fmt.Sprintf("No '%s' found nearby. Try /nearby cafes to see what's around.", name), nil
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("📍 FOUND '%s'\n\n", strings.ToUpper(name)))

	for i, e := range results {
		if i >= 5 {
			break
		}
		// Build search term with context
		var searchTerm string
		if tagsData, ok := e.Data["tags"].(map[string]interface{}); ok {
			if city, ok := tagsData["addr:city"].(string); ok && city != "" {
				searchTerm = e.Name + " " + city
			} else if street, ok := tagsData["addr:street"].(string); ok && street != "" {
				searchTerm = e.Name + " " + street
			} else {
				searchTerm = fmt.Sprintf("%s %.4f,%.4f", e.Name, e.Lat, e.Lon)
			}
		} else {
			searchTerm = fmt.Sprintf("%s %.4f,%.4f", e.Name, e.Lat, e.Lon)
		}
		mapLink := fmt.Sprintf("https://google.com/maps/search/%s", url.QueryEscape(searchTerm))
		result.WriteString(fmt.Sprintf("• %s · %s\n", e.Name, mapLink))

		// Add address
		if tagsData, ok := e.Data["tags"].(map[string]interface{}); ok {
			tags := make(map[string]string)
			for k, v := range tagsData {
				if s, ok := v.(string); ok {
					tags[k] = s
				}
			}
			if addr := formatAddress(tags); addr != "" {
				result.WriteString(fmt.Sprintf("  %s\n", addr))
			}
		}
		result.WriteString("\n")
	}

	return strings.TrimSpace(result.String()), nil
}

// NearbyWithLocation performs the actual nearby search with coordinates
func NearbyWithLocation(placeType string, lat, lon float64) (string, error) {
	placeType = strings.ToLower(strings.TrimSpace(placeType))

	// If not a valid type, search by name instead
	if !validTypes[placeType] {
		return searchByName(placeType, lat, lon)
	}

	category := normalizeType(placeType)

	// Check spatial DB first
	db := GetSpatialDB()
	cached := db.QueryPlaces(lat, lon, searchRadius, category, 20)

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
  node[%s](around:1000,%f,%f);
  way[%s](around:1000,%f,%f);
);
out center 10;
`, osmType, lat, lon, osmType, lat, lon)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.PostForm(overpassURL, url.Values{"data": {query}})
	if err != nil {
		return "Error searching for places", err
	}
	defer resp.Body.Close()

	var data OverpassResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "Error parsing results", err
	}

	if len(data.Elements) == 0 {
		return fmt.Sprintf("No %s found nearby", placeType), nil
	}

	// Cache the results in background
	go cacheOSMResults(data.Elements, category)

	return formatOSMResults(data.Elements, placeType, lat, lon), nil
}

// normalizeType converts plural/alias types to singular canonical form
func normalizeType(placeType string) string {
	aliases := map[string]string{
		"cafes": "cafe", "coffee": "cafe",
		"restaurants": "restaurant", "food": "restaurant",
		"bars": "bar",
		"pubs": "pub",
		"pharmacies": "pharmacy",
		"hospitals": "hospital",
		"banks": "bank",
		"atms": "atm",
		"grocery": "supermarket",
		"shops": "shop", "store": "shop",
		"petrol": "fuel", "gas": "fuel",
		"hotels": "hotel",
	}
	if canonical, ok := aliases[placeType]; ok {
		return canonical
	}
	return placeType
}

// cacheOSMResults stores OSM results as entities in the spatial DB
func cacheOSMResults(elements []OSMElement, category string) {
	db := GetSpatialDB()

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

		entity := &Entity{
			Type: EntityPlace,
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
func formatCachedEntities(entities []*Entity, placeType string) string {
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

		// Google Maps search with location context
		var searchTerm string
		if tagsData, ok := e.Data["tags"].(map[string]interface{}); ok {
			if city, ok := tagsData["addr:city"].(string); ok && city != "" {
				searchTerm = name + " " + city
			} else if street, ok := tagsData["addr:street"].(string); ok && street != "" {
				searchTerm = name + " " + street
			} else {
				searchTerm = fmt.Sprintf("%s %.4f,%.4f", name, e.Lat, e.Lon)
			}
		} else {
			searchTerm = fmt.Sprintf("%s %.4f,%.4f", name, e.Lat, e.Lon)
		}
		mapLink := fmt.Sprintf("https://google.com/maps/search/%s", url.QueryEscape(searchTerm))
		result.WriteString(fmt.Sprintf("• %s · %s\n", name, mapLink))

		// Extract tags
		if tagsData, ok := e.Data["tags"].(map[string]interface{}); ok {
			tags := make(map[string]string)
			for k, v := range tagsData {
				if s, ok := v.(string); ok {
					tags[k] = s
				}
			}
			if addr := formatAddress(tags); addr != "" {
				result.WriteString(fmt.Sprintf("  %s\n", addr))
			}
			if hours := tags["opening_hours"]; hours != "" {
				result.WriteString(fmt.Sprintf("  🕐 %s\n", hours))
			}
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

		// Google Maps search with location context
		var searchTerm string
		if city := el.Tags["addr:city"]; city != "" {
			searchTerm = name + " " + city
		} else if street := el.Tags["addr:street"]; street != "" {
			searchTerm = name + " " + street
		} else {
			searchTerm = fmt.Sprintf("%s %.4f,%.4f", name, elLat, elLon)
		}
		mapLink := fmt.Sprintf("https://google.com/maps/search/%s", url.QueryEscape(searchTerm))
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
