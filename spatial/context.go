package spatial

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strings"
	"time"
)

// Place represents a nearby place with structured data
type Place struct {
	Name     string  `json:"name"`
	Address  string  `json:"address,omitempty"`
	Postcode string  `json:"postcode,omitempty"`
	Phone    string  `json:"phone,omitempty"`
	Hours    string  `json:"hours,omitempty"`
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
}

// ContextData is the structured context response
type ContextData struct {
	HTML     string             `json:"html"`     // Formatted display text
	Location *LocationInfo      `json:"location"` // Where you are
	Weather  *WeatherInfo       `json:"weather"`  // Current weather
	Prayer   *PrayerInfo        `json:"prayer"`   // Prayer times
	Bus      *BusInfo           `json:"bus"`      // Nearest bus
	Places   map[string][]Place `json:"places"`   // Nearby places by category
	Agent    *AgentInfo         `json:"agent"`    // Agent for this area
}

type AgentInfo struct {
	ID        string `json:"id"`
	Status    string `json:"status"`     // active, indexing, idle
	POICount  int    `json:"poi_count"` // Places indexed
	LastIndex string `json:"last_index,omitempty"` // When last indexed
}

type LocationInfo struct {
	Name     string  `json:"name"`
	Postcode string  `json:"postcode,omitempty"`
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
}

type WeatherInfo struct {
	Temp        int    `json:"temp"`
	Condition   string `json:"condition"`
	Icon        string `json:"icon"`
	RainWarning string `json:"rain_warning,omitempty"`
}

type PrayerInfo struct {
	Current  string `json:"current,omitempty"`
	Next     string `json:"next"`
	NextTime string `json:"next_time"`
	Display  string `json:"display"` // Formatted: "Asr now · Maghrib 16:06"
}

type BusInfo struct {
	StopName string   `json:"stop_name"`
	Distance int      `json:"distance"` // meters
	Arrivals []string `json:"arrivals"` // "185 → Victoria in 3m"
}

// GetContextData returns structured context data for a location
func GetContextData(lat, lon float64) *ContextData {
	start := time.Now()
	db := Get()
	ctx := &ContextData{
		Places: make(map[string][]Place),
	}
	var htmlParts []string
	
	// Add formatted date/time for AI context
	now := time.Now()
	htmlParts = append(htmlParts, now.Format("Monday, 2 January 2006 15:04"))

	// Ensure agent exists
	agent := db.FindAgent(lat, lon, AgentRadius)
	if agent == nil {
		log.Printf("[context] Creating new agent for %.4f,%.4f", lat, lon)
		agent = db.FindOrCreateAgent(lat, lon)
	}
	
	// Agent info
	if agent != nil {
		// Count POIs in agent's area
		poiCount := len(db.Query(agent.Lat, agent.Lon, AgentRadius, EntityPlace, 500))
		status := "active"
		if s, ok := agent.Data["status"].(string); ok {
			status = s
		}
		ctx.Agent = &AgentInfo{
			ID:       agent.ID,
			Status:   status,
			POICount: poiCount,
		}
		// Last index time from UpdatedAt
		if !agent.UpdatedAt.IsZero() {
			ctx.Agent.LastIndex = agent.UpdatedAt.Format("15:04")
		}
	}

	// Location - check cache first, fetch inline if empty
	tLoc := time.Now()
	locs := db.Query(lat, lon, 500, EntityLocation, 1)
	log.Printf("[context] location cache query: %v", time.Since(tLoc))
	
	// If no cached location, fetch inline (don't make user wait for agent)
	if len(locs) == 0 {
		tFetch := time.Now()
		if loc := fetchLocation(lat, lon); loc != nil {
			db.Insert(loc)
			locs = []*Entity{loc}
			log.Printf("[context] location fetched inline: %v", time.Since(tFetch))
		}
	}
	
	if len(locs) > 0 {
		ctx.Location = &LocationInfo{
			Name: locs[0].Name,
			Lat:  lat,
			Lon:  lon,
		}
		// Extract postcode if in name
		if parts := strings.Split(locs[0].Name, ", "); len(parts) > 1 {
			ctx.Location.Postcode = parts[len(parts)-1]
		}
		htmlParts = append(htmlParts, "📍 "+locs[0].Name)
	}

	// Weather - cache only
	tWeather := time.Now()
	var headerParts []string
	var rainForecast string
	weather := db.Query(lat, lon, 10000, EntityWeather, 1)
	log.Printf("[context] weather: %v", time.Since(tWeather))
	if len(weather) > 0 {
		w := weather[0]
		ctx.Weather = &WeatherInfo{}
		
		// Get temp and build condition string (avoid "-0" display)
		var tempInt int
		if temp, ok := w.Data["temp_c"].(float64); ok {
			tempInt = int(math.Round(temp))
			if tempInt == 0 {
				tempInt = 0 // Ensure no -0
			}
			ctx.Weather.Temp = tempInt
		}
		
		// Build condition from weather code + temp
		icon := ""
		if code, ok := w.Data["weather_code"].(float64); ok {
			icon = weatherIcon(int(code))
		}
		if icon == "" {
			icon = "🌡️"
		}
		ctx.Weather.Condition = fmt.Sprintf("%s %d°C", icon, tempInt)
		
		if rf, ok := w.Data["rain_forecast"].(string); ok && rf != "" {
			ctx.Weather.RainWarning = rf
			rainForecast = rf
		}
		headerParts = append(headerParts, ctx.Weather.Condition)
	}

	// Prayer times
	prayer := db.Query(lat, lon, 50000, EntityPrayer, 1)
	if len(prayer) == 0 {
		if p := fetchPrayerTimes(lat, lon); p != nil {
			db.Insert(p)
			prayer = []*Entity{p}
		}
	}
	if len(prayer) > 0 {
		display := computePrayerDisplay(prayer[0])
		ctx.Prayer = &PrayerInfo{
			Display: display,
		}
		// Parse display to extract current/next
		if strings.Contains(display, " now") {
			parts := strings.Split(display, " ")
			if len(parts) > 0 {
				ctx.Prayer.Current = parts[0]
			}
		}
		headerParts = append(headerParts, display)
	}

	if len(headerParts) > 0 {
		htmlParts = append(htmlParts, strings.Join(headerParts, " · "))
	}
	if rainForecast != "" {
		htmlParts = append(htmlParts, rainForecast)
	}

	// Traffic disruptions
	t1 := time.Now()
	if disruption := getTrafficDisruptions(lat, lon); disruption != "" {
		htmlParts = append(htmlParts, disruption)
	}
	log.Printf("[context] disruptions: %v", time.Since(t1))

	// Bus arrivals - only use cached data, never block on TfL
	t2 := time.Now()
	if busInfo := getNearestStopCached(lat, lon); busInfo != "" {
		htmlParts = append(htmlParts, busInfo)
	}
	log.Printf("[context] bus arrivals: %v", time.Since(t2))

	// Places by category
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

	// Places - only use cached data, agents handle fetching
	tPlaces := time.Now()
	var placeParts []string
	for _, c := range categories {
		places := db.QueryPlaces(lat, lon, 500, c.category, 10)

		if len(places) > 0 {
			var categoryPlaces []Place
			for _, p := range places {
				place := Place{
					Name: p.Name,
					Lat:  p.Lat,
					Lon:  p.Lon,
				}
				if tags, ok := p.Data["tags"].(map[string]interface{}); ok {
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
					place.Address = addr
					if pc, ok := tags["addr:postcode"].(string); ok {
						place.Postcode = pc
					}
					if hours, ok := tags["opening_hours"].(string); ok {
						place.Hours = hours
					}
					if phone, ok := tags["phone"].(string); ok {
						place.Phone = phone
					} else if phone, ok := tags["contact:phone"].(string); ok {
						place.Phone = phone
					}
				}
				categoryPlaces = append(categoryPlaces, place)
			}
			ctx.Places[c.category] = categoryPlaces

			// Build HTML for this category
			var label string
			if len(categoryPlaces) == 1 {
				label = categoryPlaces[0].Name
			} else {
				label = fmt.Sprintf("%d places", len(categoryPlaces))
			}
			placeParts = append(placeParts, fmt.Sprintf("%s %s", c.icon, label))
		}
	}

	log.Printf("[context] places: %v", time.Since(tPlaces))
	if len(placeParts) > 0 {
		htmlParts = append(htmlParts, strings.Join(placeParts, " · "))
	}

	ctx.HTML = strings.Join(htmlParts, "\n")
	log.Printf("[context] GetContextData took %v", time.Since(start))
	return ctx
}

// GetContextJSON returns context as JSON string
func GetContextJSON(lat, lon float64) string {
	ctx := GetContextData(lat, lon)
	b, _ := json.Marshal(ctx)
	return string(b)
}
