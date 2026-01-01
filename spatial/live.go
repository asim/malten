package spatial

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	tflBaseURL     = "https://api.tfl.gov.uk"
	weatherURL     = "https://api.open-meteo.com/v1/forecast"
	prayerTimesURL = "https://api.aladhan.com/v1/timings"

	liveUpdateInterval = 30 * time.Second
	arrivalTTL         = 2 * time.Minute
	weatherTTL         = 10 * time.Minute
	prayerTTL          = 1 * time.Hour
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

// Live data fetch functions - called by agent loops

func fetchWeather(lat, lon float64) *Entity {
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

func fetchPrayerTimes(lat, lon float64) *Entity {
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
func fetchTransportArrivals(lat, lon float64, stopType, icon string) []*Entity {
	// Get nearby stops
	url := fmt.Sprintf("%s/StopPoint?lat=%f&lon=%f&stopTypes=%s&radius=500",
		tflBaseURL, lat, lon, stopType)
	
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Malten/1.0")
	
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	
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
		if seen[stop.CommonName] || len(entities) >= 3 {
			continue
		}
		seen[stop.CommonName] = true
		
		// Get arrivals for this stop
		arrivals := fetchStopArrivals(stop.NaptanID)
		if len(arrivals) == 0 {
			continue
		}
		
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
	
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Malten/1.0")
	
	resp, err := httpClient.Do(req)
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

// GetLiveContext queries the spatial index for live data near a location
// This is instant - no API calls, just index lookup
func GetLiveContext(lat, lon float64) string {
	db := Get()
	var parts []string
	
	// Where am I? Street + area
	location := reverseGeocode(lat, lon)
	if location != "" {
		parts = append(parts, "📍 "+location)
	}
	
	// Weather + Prayer on same line
	var header []string
	var rainForecast string
	weather := db.Query(lat, lon, 10000, EntityWeather, 1)
	if len(weather) > 0 {
		header = append(header, weather[0].Name)
		// Extract rain forecast from data
		if rf, ok := weather[0].Data["rain_forecast"].(string); ok && rf != "" {
			rainForecast = rf
		}
	}
	prayer := db.Query(lat, lon, 10000, EntityPrayer, 1)
	if len(prayer) > 0 {
		header = append(header, prayer[0].Name)
	}
	if len(header) > 0 {
		parts = append(parts, strings.Join(header, " · "))
	}
	if rainForecast != "" {
		parts = append(parts, rainForecast)
	}
	
	// Nearest bus stop with arrivals - am I AT a stop?
	nearestStop := getNearestStopWithArrivals(lat, lon)
	if nearestStop != "" {
		parts = append(parts, nearestStop)
	}
	
	// Places summary
	placesSummary := getPlacesSummary(db, lat, lon)
	if placesSummary != "" {
		parts = append(parts, placesSummary)
	}
	
	return strings.Join(parts, "\n")
}

// reverseGeocode gets street name and area
func reverseGeocode(lat, lon float64) string {
	url := fmt.Sprintf("https://nominatim.openstreetmap.org/reverse?lat=%f&lon=%f&format=json&zoom=18",
		lat, lon)
	
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Malten/1.0")
	
	resp, err := httpClient.Do(req)
	if err != nil {
		return ""
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
		return ""
	}
	
	road := data.Address.Road
	// Prefer postcode over suburb/town - clearer for users
	area := data.Address.Postcode
	if area == "" {
		area = data.Address.Suburb
	}
	if area == "" {
		area = data.Address.Town
	}
	
	if road != "" && area != "" {
		return road + ", " + area
	}
	if road != "" {
		return road
	}
	return area
}

// getNearestStopWithArrivals returns the nearest stop and its arrivals
func getNearestStopWithArrivals(lat, lon float64) string {
	db := Get()
	
	// Query quadtree for arrivals indexed by agent (100m radius)
	arrivals := db.Query(lat, lon, 100, EntityArrival, 3)
	if len(arrivals) > 0 {
		// Use cached arrival data from quadtree
		arr := arrivals[0]
		stopName, _ := arr.Data["stop_name"].(string)
		arrData, _ := arr.Data["arrivals"].([]interface{})
		
		if len(arrData) == 0 {
			return fmt.Sprintf("🚏 %s (no buses)", stopName)
		}
		
		// Format arrivals from cached data
		var lines []string
		dist := haversine(lat, lon, arr.Lat, arr.Lon) * 1000 // km to m
		if dist < 30 {
			lines = append(lines, fmt.Sprintf("🚏 At %s", stopName))
		} else {
			lines = append(lines, fmt.Sprintf("🚏 %s (%.0fm)", stopName, dist))
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
		return strings.Join(lines, "\n")
	}
	
	// Fallback: no cached data, query TfL directly (agent may not have indexed yet)
	url := fmt.Sprintf("%s/StopPoint?lat=%f&lon=%f&stopTypes=NaptanPublicBusCoachTram&radius=100",
		tflBaseURL, lat, lon)
	
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Malten/1.0")
	
	resp, err := httpClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	
	var stops struct {
		StopPoints []struct {
			NaptanID   string  `json:"naptanId"`
			CommonName string  `json:"commonName"`
			Distance   float64 `json:"distance"`
		} `json:"stopPoints"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&stops); err != nil || len(stops.StopPoints) == 0 {
		return ""
	}
	
	stop := stops.StopPoints[0]
	fetchedArrivals := fetchStopArrivals(stop.NaptanID)
	if len(fetchedArrivals) == 0 {
		return fmt.Sprintf("🚏 %s (no buses)", stop.CommonName)
	}
	
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

func getPlacesSummary(db *DB, lat, lon float64) string {
	categories := []string{"cafe", "restaurant", "pharmacy", "supermarket"}
	icons := map[string]string{"cafe": "☕", "restaurant": "🍽️", "pharmacy": "💊", "supermarket": "🛒"}
	
	var summary []string
	for _, cat := range categories {
		places := db.QueryPlaces(lat, lon, 500, cat, 10)
		if len(places) > 0 {
			icon := icons[cat]
			if len(places) == 1 {
				summary = append(summary, fmt.Sprintf("%s %s", icon, places[0].Name))
			} else {
				summary = append(summary, fmt.Sprintf("%s %d %ss", icon, len(places), cat))
			}
		}
	}
	
	if len(summary) == 0 {
		return ""
	}
	return strings.Join(summary, " · ")
}
