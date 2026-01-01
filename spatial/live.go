package spatial

import (
	"encoding/json"
	"fmt"
	"log"
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

// StartLiveIndexer starts background indexing of live data for all agents
func StartLiveIndexer() {
	go func() {
		// Initial delay to let DB load
		time.Sleep(5 * time.Second)
		
		for {
			indexLiveData()
			time.Sleep(liveUpdateInterval)
		}
	}()
	log.Println("[spatial] Live indexer started")
}

func indexLiveData() {
	db := Get()
	agents := db.ListAgents()
	
	for _, agent := range agents {
		// Index live data for each agent's territory
		go indexAgentLiveData(agent)
	}
}

func indexAgentLiveData(agent *Entity) {
	db := Get()
	
	// Weather
	if weather := fetchWeather(agent.Lat, agent.Lon); weather != nil {
		db.Insert(weather)
	}
	
	// Prayer times
	if prayer := fetchPrayerTimes(agent.Lat, agent.Lon); prayer != nil {
		db.Insert(prayer)
	}
	
	// Bus arrivals
	arrivals := fetchBusArrivals(agent.Lat, agent.Lon)
	for _, arr := range arrivals {
		db.Insert(arr)
	}
}

func fetchWeather(lat, lon float64) *Entity {
	url := fmt.Sprintf("%s?latitude=%.2f&longitude=%.2f&current=temperature_2m,weather_code&timezone=auto",
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
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil
	}
	
	expiry := time.Now().Add(weatherTTL)
	return &Entity{
		ID:   GenerateID(EntityWeather, lat, lon, "weather"),
		Type: EntityWeather,
		Name: fmt.Sprintf("%s %.0f°C", weatherIcon(data.Current.WeatherCode), data.Current.Temperature),
		Lat:  lat,
		Lon:  lon,
		Data: map[string]interface{}{
			"temp_c":       data.Current.Temperature,
			"weather_code": data.Current.WeatherCode,
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

func fetchBusArrivals(lat, lon float64) []*Entity {
	// Get nearby stops
	url := fmt.Sprintf("%s/StopPoint?lat=%f&lon=%f&stopTypes=NaptanPublicBusCoachTram&radius=500",
		tflBaseURL, lat, lon)
	
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
			Name: fmt.Sprintf("🚌 %s: %s", stop.CommonName, strings.Join(times, ", ")),
			Lat:  stop.Lat,
			Lon:  stop.Lon,
			Data: map[string]interface{}{
				"stop_id":   stop.NaptanID,
				"stop_name": stop.CommonName,
				"arrivals":  arrivals,
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

// GetLiveContext queries the spatial index for live data near a location
// This is instant - no API calls, just index lookup
func GetLiveContext(lat, lon float64) string {
	db := Get()
	var parts []string
	
	// Weather (nearest)
	weather := db.Query(lat, lon, 10000, EntityWeather, 1)
	if len(weather) > 0 {
		parts = append(parts, weather[0].Name)
	}
	
	// Prayer (nearest)
	prayer := db.Query(lat, lon, 10000, EntityPrayer, 1)
	if len(prayer) > 0 {
		if len(parts) > 0 {
			parts[0] = parts[0] + " · " + prayer[0].Name
		} else {
			parts = append(parts, prayer[0].Name)
		}
	}
	
	// Bus arrivals (nearby)
	arrivals := db.Query(lat, lon, 500, EntityArrival, 3)
	for _, arr := range arrivals {
		parts = append(parts, arr.Name)
	}
	
	// Places summary
	placesSummary := getPlacesSummary(db, lat, lon)
	if placesSummary != "" {
		parts = append(parts, placesSummary)
	}
	
	return strings.Join(parts, "\n")
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
