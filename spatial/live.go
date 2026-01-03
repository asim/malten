package spatial

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	tflBaseURL     = "https://api.tfl.gov.uk"
	weatherURL     = "https://api.open-meteo.com/v1/forecast"
	prayerTimesURL = "https://api.aladhan.com/v1/timings"

	liveUpdateInterval = 30 * time.Second
	arrivalTTL         = 5 * time.Minute
	weatherTTL         = 10 * time.Minute
	prayerTTL          = 1 * time.Hour
	newsTTL            = 30 * time.Minute
	disruptionTTL      = 10 * time.Minute // Cache traffic disruptions
	
	bbcUKRSS           = "https://feeds.bbci.co.uk/news/uk/rss.xml"
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

// Live data fetch functions - called by agent loops

func fetchWeather(lat, lon float64) *Entity {
	// Check spatial cache first - weather valid for ~5km
	db := Get()
	cached := db.Query(lat, lon, 5000, EntityWeather, 1)
	if len(cached) > 0 && cached[0].ExpiresAt != nil && time.Now().Before(*cached[0].ExpiresAt) {
		return nil // Already have fresh data nearby
	}
	
	url := fmt.Sprintf("%s?latitude=%.2f&longitude=%.2f&current=temperature_2m,weather_code&hourly=precipitation_probability&timezone=auto&forecast_hours=6",
		weatherURL, lat, lon)
	
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Malten/1.0")
	
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	
	var data struct {
		Current struct {
			Temperature float64 `json:"temperature_2m"`
			WeatherCode int     `json:"weather_code"`
		} `json:"current"`
		Hourly struct {
			Time       []string `json:"time"`
			PrecipProb []int    `json:"precipitation_probability"`
		} `json:"hourly"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil
	}
	
	// Check rain forecast
	rainForecast := ""
	for i, prob := range data.Hourly.PrecipProb {
		if prob >= 50 && i < len(data.Hourly.Time) {
			// Parse time to get hour
			t, _ := time.Parse("2006-01-02T15:04", data.Hourly.Time[i])
			hour := t.Format("15:04")
			if i == 0 {
				rainForecast = fmt.Sprintf("🌧️ Rain likely now (%d%%)", prob)
			} else {
				rainForecast = fmt.Sprintf("🌧️ Rain at %s (%d%%)", hour, prob)
			}
			break
		}
	}
	
	name := fmt.Sprintf("%s %.0f°C", weatherIcon(data.Current.WeatherCode), data.Current.Temperature)
	
	expiry := time.Now().Add(weatherTTL)
	return &Entity{
		ID:   GenerateID(EntityWeather, lat, lon, "weather"),
		Type: EntityWeather,
		Name: name,
		Lat:  lat,
		Lon:  lon,
		Data: map[string]interface{}{
			"temp_c":        data.Current.Temperature,
			"weather_code":  data.Current.WeatherCode,
			"rain_forecast": rainForecast,
		},
		ExpiresAt: &expiry,
	}
}

// computePrayerDisplay calculates current/next prayer from stored timings at query time
func computePrayerDisplay(e *Entity) string {
	if e == nil || e.Data == nil {
		return ""
	}
	
	timingsRaw, ok := e.Data["timings"]
	if !ok {
		// No timings stored - return cached name (may be stale)
		return e.Name
	}
	
	// Convert timings to map[string]string
	timings := make(map[string]string)
	switch t := timingsRaw.(type) {
	case map[string]interface{}:
		for k, v := range t {
			if s, ok := v.(string); ok {
				timings[k] = s
			}
		}
	case map[string]string:
		timings = t
	default:
		return e.Name
	}
	
	// Prayer times in order (Sunrise is not a prayer but marks end of Fajr)
	prayers := []string{"Fajr", "Sunrise", "Dhuhr", "Asr", "Maghrib", "Isha"}
	nowStr := time.Now().Format("15:04")
	
	var current, next, nextTime string
	for i, p := range prayers {
		pTime := timings[p]
		if len(pTime) > 5 {
			pTime = pTime[:5] // strip seconds if present
		}
		if pTime > nowStr {
			// Found next prayer/event
			if p == "Sunrise" {
				// Before sunrise = Fajr is current
				current = "Fajr"
				next = "Dhuhr"
				nextTime = timings["Dhuhr"]
			} else if p == "Dhuhr" && i > 0 && prayers[i-1] == "Sunrise" {
				// After sunrise but before Dhuhr - no current prayer
				current = ""
				next = p
				nextTime = pTime
			} else {
				if i > 0 && prayers[i-1] != "Sunrise" {
					current = prayers[i-1]
				}
				next = p
				nextTime = pTime
			}
			break
		}
	}
	if next == "" {
		// After Isha time - Isha is current until Fajr
		current = "Isha"
		next = "Fajr"
		nextTime = timings["Fajr"]
	}
	if len(nextTime) > 5 {
		nextTime = nextTime[:5]
	}
	
	if current != "" {
		// For Fajr, show when it ends (before sunrise - when threads become distinguishable)
		if current == "Fajr" {
			sunrise := timings["Sunrise"]
			if len(sunrise) > 5 {
				sunrise = sunrise[:5]
			}
			// Fajr ends before sunrise - calculate end time (10 min before sunrise)
			if sunriseTime, err := time.Parse("15:04", sunrise); err == nil {
				now := time.Now()
				sunriseToday := time.Date(now.Year(), now.Month(), now.Day(), sunriseTime.Hour(), sunriseTime.Minute(), 0, 0, now.Location())
				fajrEnd := sunriseToday.Add(-10 * time.Minute) // Fajr ends 10 min before sunrise
				fajrEndStr := fajrEnd.Format("15:04")
				
				if fajrEnd.Sub(now) <= 15*time.Minute && fajrEnd.Sub(now) > 0 {
					return fmt.Sprintf("🕌 %s ending %s · %s %s", current, fajrEndStr, next, nextTime)
				}
				return fmt.Sprintf("🕌 %s ends %s · %s %s", current, fajrEndStr, next, nextTime)
			}
			return fmt.Sprintf("🕌 %s · %s %s", current, next, nextTime)
		}
		return fmt.Sprintf("🕌 %s now · %s %s", current, next, nextTime)
	}
	return fmt.Sprintf("🕌 %s %s", next, nextTime)
}

func fetchPrayerTimes(lat, lon float64) *Entity {
	// Check spatial cache first - prayer times valid for ~50km (same city)
	db := Get()
	cached := db.Query(lat, lon, 50000, EntityPrayer, 1)
	if len(cached) > 0 && cached[0].ExpiresAt != nil && time.Now().Before(*cached[0].ExpiresAt) {
		return nil // Already have fresh data nearby
	}
	
	log.Printf("[prayer] Fetching prayer times for %.4f,%.4f", lat, lon)
	now := time.Now()
	url := fmt.Sprintf("%s/%s?latitude=%.2f&longitude=%.2f&method=2",
		prayerTimesURL, now.Format("02-01-2006"), lat, lon)
	
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Malten/1.0")
	
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		return nil
	}
	
	var data struct {
		Data struct {
			Timings map[string]string `json:"timings"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil
	}
	
	// Calculate current and next prayer
	prayers := []string{"Fajr", "Sunrise", "Dhuhr", "Asr", "Maghrib", "Isha"}
	nowStr := now.Format("15:04")
	
	var current, next, nextTime string
	for i, p := range prayers {
		pTime := data.Data.Timings[p]
		if pTime > nowStr {
			if i > 0 {
				current = prayers[i-1]
			}
			if p == "Sunrise" {
				next = "Dhuhr"
				nextTime = data.Data.Timings["Dhuhr"]
			} else {
				next = p
				nextTime = pTime
			}
			break
		}
	}
	if next == "" {
		current = "Isha"
		next = "Fajr"
		nextTime = data.Data.Timings["Fajr"]
	}
	if current == "Sunrise" {
		current = "Fajr"
	}
	
	var name string
	if current != "" {
		name = fmt.Sprintf("🕌 %s now · %s %s", current, next, nextTime)
	} else {
		name = fmt.Sprintf("🕌 %s %s", next, nextTime)
	}
	
	expiry := time.Now().Add(prayerTTL)
	return &Entity{
		ID:   GenerateID(EntityPrayer, lat, lon, "prayer"),
		Type: EntityPrayer,
		Name: name,
		Lat:  lat,
		Lon:  lon,
		Data: map[string]interface{}{
			"timings": data.Data.Timings,
			"current": current,
			"next":    next,
		},
		ExpiresAt: &expiry,
	}
}

// fetchBusArrivals is deprecated, use fetchTransportArrivals
func fetchBusArrivals(lat, lon float64) []*Entity {
	return fetchTransportArrivals(lat, lon, "NaptanPublicBusCoachTram", "🚌")
}

// fetchTransportArrivals gets arrivals for any TfL stop type
// stopType: NaptanPublicBusCoachTram, NaptanMetroStation, NaptanRailStation
// icon: 🚌 🚇 🚆
// fetchTransportArrivals returns:
//   - slice of entities if new arrivals were fetched
//   - empty slice if API returned no arrivals (caller should extend TTL)
//   - nil if skipped because fresh cache exists (caller should not extend TTL)
func fetchTransportArrivals(lat, lon float64, stopType, icon string) []*Entity {
	// TfL only works for London - check region
	if !IsLondon(lat, lon) {
		return nil // Outside London - don't query TfL
	}
	
	// Check if we have fresh arrivals in this area already
	db := Get()
	cached := db.Query(lat, lon, 500, EntityArrival, 3)
	freshCount := 0
	for _, arr := range cached {
		if arr.ExpiresAt != nil && time.Now().Before(*arr.ExpiresAt) {
			freshCount++
		}
	}
	if freshCount >= 2 {
		log.Printf("[transport] Skipping fetch for %.4f,%.4f - have %d fresh arrivals", lat, lon, freshCount)
		return nil // nil = skipped, don't extend TTL
	}
	log.Printf("[transport] Fetching %s arrivals for %.4f,%.4f (cached fresh: %d)", stopType, lat, lon, freshCount)
	
	// Get nearby stops - use rate limiter to prevent hammering TfL
	url := fmt.Sprintf("%s/StopPoint?lat=%f&lon=%f&stopTypes=%s&radius=500",
		tflBaseURL, lat, lon, stopType)
	
	var resp *http.Response
	var fetchErr error
	
	err := TfLRateLimitedCall(func() error {
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("User-Agent", "Malten/1.0")
		
		var err error
		resp, err = httpClient.Do(req)
		fetchErr = err
		return err
	})
	
	if err != nil || fetchErr != nil {
		log.Printf("[transport] API error for %s: %v", stopType, err)
		return []*Entity{} // empty = extend TTL
	}
	defer resp.Body.Close()
	
	if resp.StatusCode == 429 {
		log.Printf("[transport] Rate limited (429) for %s", stopType)
		return []*Entity{} // empty = extend TTL
	}
	if resp.StatusCode != 200 {
		log.Printf("[transport] API returned %d for %s", resp.StatusCode, stopType)
		return []*Entity{} // empty = extend TTL
	}
	
	var stops struct {
		StopPoints []struct {
			NaptanID   string  `json:"naptanId"`
			CommonName string  `json:"commonName"`
			Lat        float64 `json:"lat"`
			Lon        float64 `json:"lon"`
		} `json:"stopPoints"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&stops); err != nil {
		return nil
	}
	
	var entities []*Entity
	seen := make(map[string]bool)
	
	for _, stop := range stops.StopPoints {
		if seen[stop.CommonName] {
			log.Printf("[transport] Skipping %s (already seen)", stop.CommonName)
			continue
		}
		if len(entities) >= 3 {
			log.Printf("[transport] Stopping at %s (have 3 entities)", stop.CommonName)
			break
		}
		
		// Get arrivals for this stop
		arrivals := fetchStopArrivals(stop.NaptanID)
		if len(arrivals) == 0 {
			log.Printf("[transport] Stop %s (%s) has no arrivals", stop.CommonName, stop.NaptanID)
			continue
		}
		// Only mark as seen once we have arrivals (so we don't skip the other direction)
		seen[stop.CommonName] = true
		log.Printf("[transport] Stop %s (%s): %d arrivals", stop.CommonName, stop.NaptanID, len(arrivals))
		
		// Format arrivals
		var times []string
		for i, arr := range arrivals {
			if i >= 2 {
				break
			}
			times = append(times, fmt.Sprintf("%s→%s %dm",
				arr.Line, shortDest(arr.Destination), arr.Minutes))
		}
		
		expiry := time.Now().Add(arrivalTTL)
		entities = append(entities, &Entity{
			ID:   GenerateID(EntityArrival, stop.Lat, stop.Lon, stop.NaptanID),
			Type: EntityArrival,
			Name: fmt.Sprintf("%s %s: %s", icon, stop.CommonName, strings.Join(times, ", ")),
			Lat:  stop.Lat,
			Lon:  stop.Lon,
			Data: map[string]interface{}{
				"stop_id":    stop.NaptanID,
				"stop_name":  stop.CommonName,
				"stop_type":  stopType,
				"arrivals":   arrivals,
			},
			ExpiresAt: &expiry,
		})
	}
	
	return entities
}

type busArrival struct {
	Line        string `json:"line"`
	Destination string `json:"destination"`
	Minutes     int    `json:"minutes"`
}

func fetchStopArrivals(naptanID string) []busArrival {
	url := fmt.Sprintf("%s/StopPoint/%s/Arrivals", tflBaseURL, naptanID)
	
	var resp *http.Response
	
	err := TfLRateLimitedCall(func() error {
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("User-Agent", "Malten/1.0")
		
		var err error
		resp, err = httpClient.Do(req)
		return err
	})
	
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	
	var data []struct {
		LineName        string `json:"lineName"`
		DestinationName string `json:"destinationName"`
		TimeToStation   int    `json:"timeToStation"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil
	}
	
	// Sort by time
	sort.Slice(data, func(i, j int) bool {
		return data[i].TimeToStation < data[j].TimeToStation
	})
	
	var arrivals []busArrival
	for _, d := range data {
		arrivals = append(arrivals, busArrival{
			Line:        d.LineName,
			Destination: d.DestinationName,
			Minutes:     d.TimeToStation / 60,
		})
	}
	return arrivals
}

func weatherIcon(code int) string {
	switch {
	case code == 0:
		return "☀️"
	case code <= 3:
		return "⛅"
	case code <= 49:
		return "🌫️"
	case code <= 69:
		return "🌧️"
	case code <= 79:
		return "❄️"
	case code <= 99:
		return "⛈️"
	default:
		return "🌡️"
	}
}

func shortDest(dest string) string {
	dest = strings.TrimSuffix(dest, " Bus Station")
	dest = strings.TrimSuffix(dest, " Station")
	if len(dest) > 15 {
		if idx := strings.Index(dest, " "); idx > 0 {
			return dest[:idx]
		}
	}
	return dest
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


// fetchLocation reverse geocodes and returns an entity for caching
// Locations are cached for 500m radius - same street basically
func fetchLocation(lat, lon float64) *Entity {
	// Check cache first
	db := Get()
	cached := db.Query(lat, lon, 500, EntityLocation, 1)
	if len(cached) > 0 && cached[0].ExpiresAt != nil && time.Now().Before(*cached[0].ExpiresAt) {
		return nil // Already have fresh data
	}
	
	url := fmt.Sprintf("https://nominatim.openstreetmap.org/reverse?lat=%f&lon=%f&format=json&zoom=18",
		lat, lon)
	
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Malten/1.0")
	
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	
	var data struct {
		Address struct {
			Road     string `json:"road"`
			Suburb   string `json:"suburb"`
			Town     string `json:"town"`
			Postcode string `json:"postcode"`
		} `json:"address"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil
	}
	
	road := data.Address.Road
	area := data.Address.Postcode
	if area == "" {
		area = data.Address.Suburb
	}
	if area == "" {
		area = data.Address.Town
	}
	
	var name string
	if road != "" && area != "" {
		name = road + ", " + area
	} else if road != "" {
		name = road
	} else {
		name = area
	}
	
	if name == "" {
		return nil
	}
	
	// Cache for 1 hour - locations don't change
	expiry := time.Now().Add(1 * time.Hour)
	return &Entity{
		ID:        GenerateID(EntityLocation, lat, lon, "location"),
		Type:      EntityLocation,
		Name:      name,
		Lat:       lat,
		Lon:       lon,
		Data:      map[string]interface{}{"road": road, "area": area, "postcode": data.Address.Postcode},
		ExpiresAt: &expiry,
	}
}

// getNearestStopWithArrivals returns the nearest stop and its arrivals
func getNearestStopWithArrivals(lat, lon float64) string {
	db := Get()
	
	// Query quadtree for arrivals - allow stale data up to 10 minutes past expiry
	// This ensures buses don't disappear just because a refresh failed
	arrivals := db.QueryWithMaxAge(lat, lon, 500, EntityArrival, 5, 600)
	
	// Check if any are actually fresh (not stale)
	hasFresh := false
	for _, arr := range arrivals {
		if arr.ExpiresAt != nil && time.Now().Before(*arr.ExpiresAt) {
			hasFresh = true
			break
		}
	}
	
	// Find first stop with actual arrivals
	for _, arr := range arrivals {
		stopName, _ := arr.Data["stop_name"].(string)
		arrData, _ := arr.Data["arrivals"].([]interface{})
		
		if len(arrData) == 0 {
			continue // Skip stops with no arrivals
		}
		
		// Check if this particular record is stale
		isStale := arr.ExpiresAt != nil && time.Now().After(*arr.ExpiresAt)
		
		// Format arrivals from cached data
		var lines []string
		dist := haversine(lat, lon, arr.Lat, arr.Lon) * 1000 // km to m
		stopLabel := stopName
		if dist >= 30 {
			stopLabel = fmt.Sprintf("%s (%.0fm)", stopName, dist)
		}
		if isStale {
			lines = append(lines, fmt.Sprintf("🚏 %s ⏳", stopLabel)) // Clock shows data may be stale
		} else if dist < 30 {
			lines = append(lines, fmt.Sprintf("🚏 At %s", stopName))
		} else {
			lines = append(lines, fmt.Sprintf("🚏 %s", stopLabel))
		}
		
		for i, a := range arrData {
			if i >= 3 {
				break
			}
			if amap, ok := a.(map[string]interface{}); ok {
				line, _ := amap["line"].(string)
				dest, _ := amap["destination"].(string)
				mins, _ := amap["minutes"].(float64)
				if mins <= 1 {
					lines = append(lines, fmt.Sprintf("   %s → %s arriving now", line, shortDest(dest)))
				} else {
					lines = append(lines, fmt.Sprintf("   %s → %s in %.0fm", line, shortDest(dest), mins))
				}
			}
		}
		
		// If we used stale data, try to refresh in background
		if isStale && !hasFresh {
			go func(stopID string) {
				if arrs := fetchStopArrivals(stopID); len(arrs) > 0 {
					expiry := time.Now().Add(arrivalTTL)
					db.Insert(&Entity{
						ID:        GenerateID(EntityArrival, lat, lon, stopID),
						Type:      EntityArrival,
						Name:      fmt.Sprintf("🚌 %s", stopName),
						Lat:       arr.Lat,
						Lon:       arr.Lon,
						Data:      map[string]interface{}{"stop_id": stopID, "stop_name": stopName, "arrivals": arrs},
						ExpiresAt: &expiry,
					})
					log.Printf("[arrivals] Background refresh for %s complete", stopName)
				}
			}(arr.Data["stop_id"].(string))
		}
		
		return strings.Join(lines, "\n")
	}
	
	// Fallback: no cached data with arrivals, query TfL directly (rate limited)
	log.Printf("[context] No cached arrivals at %.4f,%.4f - querying TfL directly", lat, lon)
	
	stopUrl := fmt.Sprintf("%s/StopPoint?lat=%f&lon=%f&stopTypes=NaptanPublicBusCoachTram&radius=500",
		tflBaseURL, lat, lon)
	
	var resp *http.Response
	
	err := TfLRateLimitedCall(func() error {
		req, _ := http.NewRequest("GET", stopUrl, nil)
		req.Header.Set("User-Agent", "Malten/1.0")
		
		var err error
		resp, err = httpClient.Do(req)
		return err
	})
	
	if err != nil {
		log.Printf("[context] TfL StopPoint query failed: %v", err)
		return ""
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		log.Printf("[context] TfL StopPoint returned status %d", resp.StatusCode)
		return ""
	}
	
	var stops struct {
		StopPoints []struct {
			NaptanID   string  `json:"naptanId"`
			CommonName string  `json:"commonName"`
			Distance   float64 `json:"distance"`
			Lat        float64 `json:"lat"`
			Lon        float64 `json:"lon"`
		} `json:"stopPoints"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&stops); err != nil {
		log.Printf("[context] TfL StopPoint decode failed: %v", err)
		return ""
	}
	if len(stops.StopPoints) == 0 {
		log.Printf("[context] TfL returned no stops near %.4f,%.4f", lat, lon)
		return ""
	}
	
	log.Printf("[context] TfL returned %d stops", len(stops.StopPoints))
	
	// Find first stop with arrivals
	for _, stop := range stops.StopPoints {
		fetchedArrivals := fetchStopArrivals(stop.NaptanID)
		if len(fetchedArrivals) == 0 {
			continue
		}
		
		// Cache these arrivals so we don't hit TfL again
		expiry := time.Now().Add(arrivalTTL)
		stopLat, stopLon := stop.Lat, stop.Lon
		if stopLat == 0 {
			// Fallback if TfL didn't return coords (shouldn't happen)
			stopLat, stopLon = lat, lon
		}
		arrEntity := &Entity{
			ID:   GenerateID(EntityArrival, stopLat, stopLon, stop.NaptanID),
			Type: EntityArrival,
			Name: fmt.Sprintf("🚌 %s", stop.CommonName),
			Lat:  stopLat,
			Lon:  stopLon,
			Data: map[string]interface{}{
				"stop_id":    stop.NaptanID,
				"stop_name":  stop.CommonName,
				"stop_type":  "NaptanPublicBusCoachTram",
				"arrivals":   fetchedArrivals,
			},
			ExpiresAt: &expiry,
		}
		db.Insert(arrEntity)
		
		// Format: "🚏 Whitton Station" then list next buses
		var lines []string
		if stop.Distance < 30 {
			lines = append(lines, fmt.Sprintf("🚏 At %s", stop.CommonName))
		} else {
			lines = append(lines, fmt.Sprintf("🚏 %s (%.0fm)", stop.CommonName, stop.Distance))
		}
		
		// Show next 3 arrivals
		for i, arr := range fetchedArrivals {
			if i >= 3 {
				break
			}
			if arr.Minutes <= 1 {
				lines = append(lines, fmt.Sprintf("   %s → %s arriving now", arr.Line, shortDest(arr.Destination)))
			} else {
				lines = append(lines, fmt.Sprintf("   %s → %s in %dm", arr.Line, shortDest(arr.Destination), arr.Minutes))
			}
		}
		return strings.Join(lines, "\n")
	}
	
	// No stops with arrivals found
	log.Printf("[context] No stops with arrivals found")
	return ""
}

func getPlacesSummary(db *DB, lat, lon float64) string {
	categories := []string{"cafe", "restaurant", "pharmacy", "supermarket"}
	icons := map[string]string{"cafe": "☕", "restaurant": "🍽️", "pharmacy": "💊", "supermarket": "🛒"}
	
	var summary []string
	for _, cat := range categories {
		places := db.QueryPlaces(lat, lon, 500, cat, 10)
		if len(places) > 0 {
			icon := icons[cat]
			var placesData []string
			for _, p := range places {
				placesData = append(placesData, formatPlaceData(p))
			}
			summary = append(summary, fmt.Sprintf("%s {%s}", icon, strings.Join(placesData, ";;")))
		}
	}
	
	if len(summary) == 0 {
		return ""
	}
	return strings.Join(summary, " · ")
}

// getPlacesSummaryOrFetch returns cached places, fetching any missing categories
func getPlacesSummaryOrFetch(db *DB, lat, lon float64) string {
	categories := []struct {
		osmTag   string
		category string
		icon     string
	}{
		{"amenity=cafe", "cafe", "☕"},
		{"amenity=restaurant", "restaurant", "🍽️"},
		{"amenity=pharmacy", "pharmacy", "💊"},
		{"shop=supermarket", "supermarket", "🛒"},
	}
	
	var results []string
	for _, c := range categories {
		// Check cache first
		places := db.QueryPlaces(lat, lon, 500, c.category, 10)
		
		// If cache empty for this category, fetch on demand
		if len(places) == 0 {
			fetched := fetchPlacesNow(lat, lon, 500, c.osmTag, c.category, 10)
			for _, p := range fetched {
				db.Insert(p)
			}
			places = fetched
		}
		
		if len(places) > 0 {
			var placesData []string
			for _, p := range places {
				placesData = append(placesData, formatPlaceData(p))
			}
			results = append(results, fmt.Sprintf("%s {%s}", c.icon, strings.Join(placesData, ";;")))
		}
	}
	
	if len(results) == 0 {
		return ""
	}
	return strings.Join(results, " · ")
}

// fetchPlacesNow fetches places immediately from Overpass
func fetchPlacesNow(lat, lon, radius float64, osmTag, category string, limit int) []*Entity {
	query := fmt.Sprintf(`[out:json][timeout:10];
(
  node[%s](around:%.0f,%f,%f);
  way[%s](around:%.0f,%f,%f);
);
out center %d;`, osmTag, radius, lat, lon, osmTag, radius, lat, lon, limit)
	
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.PostForm("https://overpass-api.de/api/interpreter", url.Values{"data": {query}})
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		return nil
	}
	
	var data struct {
		Elements []struct {
			ID     int64             `json:"id"`
			Lat    float64           `json:"lat"`
			Lon    float64           `json:"lon"`
			Center *struct {
				Lat float64 `json:"lat"`
				Lon float64 `json:"lon"`
			} `json:"center,omitempty"`
			Tags map[string]string `json:"tags"`
		} `json:"elements"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil
	}
	
	var entities []*Entity
	for _, el := range data.Elements {
		elat, elon := el.Lat, el.Lon
		if elat == 0 && el.Center != nil {
			elat, elon = el.Center.Lat, el.Center.Lon
		}
		if elat == 0 || el.Tags["name"] == "" {
			continue
		}
		
		tags := make(map[string]interface{})
		for k, v := range el.Tags {
			tags[k] = v
		}
		
		entities = append(entities, &Entity{
			Type: EntityPlace,
			Name: el.Tags["name"],
			Lat:  elat,
			Lon:  elon,
			Data: map[string]interface{}{
				"category": category,
				"tags":     tags,
				"osm_id":   el.ID,
			},
		})
	}
	return entities
}

// formatPlaceData creates a compact data string with all place info
func formatPlaceData(e *Entity) string {
	parts := []string{e.Name}
	
	// Extract from tags
	if tags, ok := e.Data["tags"].(map[string]interface{}); ok {
		var addr string
		if num, ok := tags["addr:housenumber"].(string); ok {
			addr = num
		}
		if street, ok := tags["addr:street"].(string); ok {
			if addr != "" {
				addr += " "
			}
			addr += street
		}
		if addr != "" {
			parts = append(parts, addr)
		}
		if pc, ok := tags["addr:postcode"].(string); ok {
			parts = append(parts, pc)
		}
		if hours, ok := tags["opening_hours"].(string); ok {
			parts = append(parts, "🕒"+hours)
		}
		if phone, ok := tags["phone"].(string); ok {
			parts = append(parts, "📞"+phone)
		} else if phone, ok := tags["contact:phone"].(string); ok {
			parts = append(parts, "📞"+phone)
		}
	}
	
	// Add map link with name for better search
	parts = append(parts, fmt.Sprintf("https://maps.google.com/maps/search/%s/@%f,%f,17z", url.QueryEscape(e.Name), e.Lat, e.Lon))
	
	return strings.Join(parts, "|")
}

// getTrafficDisruptions returns cached disruptions or fetches new ones
func getTrafficDisruptions(lat, lon float64) string {
	db := Get()
	
	// Check cache first - 10km radius for disruptions
	cached := db.Query(lat, lon, 10000, EntityDisruption, 1)
	for _, d := range cached {
		if d.ExpiresAt != nil && time.Now().Before(*d.ExpiresAt) {
			return d.Name
		}
	}
	
	// Fetch fresh disruptions
	return fetchTrafficDisruptions(lat, lon)
}

// fetchTrafficDisruptions gets nearby road disruptions from TfL and caches them
func fetchTrafficDisruptions(lat, lon float64) string {
	db := Get()
	disruptionUrl := "https://api.tfl.gov.uk/Road/all/Disruption"
	
	var resp *http.Response
	
	err := TfLRateLimitedCall(func() error {
		req, _ := http.NewRequest("GET", disruptionUrl, nil)
		req.Header.Set("User-Agent", "Malten/1.0")
		
		var err error
		resp, err = httpClient.Do(req)
		return err
	})
	
	if err != nil {
		log.Printf("[disruption] TfL API error: %v", err)
		return ""
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		log.Printf("[disruption] TfL API returned status %d", resp.StatusCode)
		return ""
	}
	
	var disruptions []struct {
		Severity  string `json:"severity"`
		Category  string `json:"category"`
		Comments  string `json:"comments"`
		Location  string `json:"location"`
		Geography struct {
			Type        string    `json:"type"`
			Coordinates []float64 `json:"coordinates"`
		} `json:"geography"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&disruptions); err != nil {
		log.Printf("[disruption] Decode error: %v", err)
		return ""
	}
	
	// Find disruptions within 5km, prioritize serious ones
	type nearby struct {
		dist     float64
		severity string
		text     string
		lat      float64
		lon      float64
	}
	var found []nearby
	
	for _, d := range disruptions {
		if d.Geography.Type != "Point" || len(d.Geography.Coordinates) < 2 {
			continue
		}
		dlon, dlat := d.Geography.Coordinates[0], d.Geography.Coordinates[1]
		dist := haversine(lat, lon, dlat, dlon)
		
		if dist < 5 { // within 5km
			// Only show serious/moderate or collisions
			if d.Severity == "Serious" || d.Severity == "Moderate" || d.Category == "Collisions" {
				text := d.Comments
				if len(text) > 80 {
					text = text[:80] + "..."
				}
				found = append(found, nearby{dist: dist, severity: d.Severity, text: text, lat: dlat, lon: dlon})
			}
		}
	}
	
	if len(found) == 0 {
		return ""
	}
	
	// Return closest serious one
	var best nearby
	for _, f := range found {
		if best.text == "" || f.dist < best.dist {
			best = f
		}
	}
	
	icon := "🚧"
	if strings.Contains(strings.ToLower(best.text), "collision") || strings.Contains(strings.ToLower(best.text), "accident") {
		icon = "🚨"
	}
	
	result := fmt.Sprintf("%s %.1fkm: %s", icon, best.dist, best.text)
	
	// Cache this disruption
	expiry := time.Now().Add(disruptionTTL)
	db.Insert(&Entity{
		ID:        GenerateID(EntityDisruption, best.lat, best.lon, best.text[:min(20, len(best.text))]),
		Type:      EntityDisruption,
		Name:      result,
		Lat:       best.lat,
		Lon:       best.lon,
		Data:      map[string]interface{}{"severity": best.severity, "text": best.text},
		ExpiresAt: &expiry,
	})
	
	return result
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// fetchBreakingNews gets top UK headline from BBC RSS
// GetBreakingNews returns the top UK headline (cached 30min)
func GetBreakingNews() string {
	if news := fetchBreakingNews(); news != nil {
		Get().Insert(news)
		return news.Name
	}
	return ""
}

func fetchBreakingNews() *Entity {
	// Check cache first (global, not location-based)
	db := Get()
	news := db.Query(0, 0, 1000000, EntityNews, 1) // Global query
	for _, n := range news {
		if n.ExpiresAt != nil && time.Now().Before(*n.ExpiresAt) {
			return n
		}
	}
	
	req, _ := http.NewRequest("GET", bbcUKRSS, nil)
	req.Header.Set("User-Agent", "Malten/1.0")
	
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	
	var rss struct {
		Channel struct {
			Items []struct {
				Title   string `xml:"title"`
				Link    string `xml:"link"`
				PubDate string `xml:"pubDate"`
			} `xml:"item"`
		} `xml:"channel"`
	}
	
	if err := xml.NewDecoder(resp.Body).Decode(&rss); err != nil {
		return nil
	}
	
	if len(rss.Channel.Items) == 0 {
		return nil
	}
	
	top := rss.Channel.Items[0]
	expiry := time.Now().Add(newsTTL)
	
	// Clean up the link (remove tracking params)
	link := top.Link
	if idx := strings.Index(link, "?"); idx > 0 {
		link = link[:idx]
	}
	
	return &Entity{
		ID:        "news-uk-headline",
		Type:      EntityNews,
		Name:      "📰 " + top.Title + "\n" + link,
		Lat:       0, // Global
		Lon:       0,
		Data:      map[string]interface{}{"source": "BBC UK", "pubDate": top.PubDate, "link": link},
		ExpiresAt: &expiry,
	}
}
