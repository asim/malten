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
		if agentData := agent.GetAgentData(); agentData != nil && agentData.Status != "" {
			status = agentData.Status
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

	// Location - find nearest location point or create one
	// GPS jitter tolerance: 10m - if we have a point within 10m, use it
	// Otherwise fetch from Nominatim and store a NEW point at these exact coords
	tLoc := time.Now()
	loc := db.GetNearestLocation(lat, lon, 10) // 10m tolerance for GPS jitter
	log.Printf("[context] location lookup: %v", time.Since(tLoc))
	
	var locationName string
	if loc != nil {
		locationName = loc.Name
	} else {
		// No point nearby - fetch and create one at THESE coords
		newLoc := fetchLocation(lat, lon)
		if newLoc != nil {
			locationName = newLoc.Name
		}
	}
	
	if locationName != "" {
		ctx.Location = &LocationInfo{
			Name: locationName,
			Lat:  lat,
			Lon:  lon,
		}
		// Extract postcode if in name
		if parts := strings.Split(locationName, ", "); len(parts) > 1 {
			ctx.Location.Postcode = parts[len(parts)-1]
		}
		htmlParts = append(htmlParts, "📍 "+locationName)
	}

	// Weather - cache only
	tWeather := time.Now()
	var headerParts []string
	var rainForecast string
	// Weather query radius must match fetch radius (5km) to avoid stale data from further away
	weather := db.Query(lat, lon, 5000, EntityWeather, 1)
	log.Printf("[context] weather: %v", time.Since(tWeather))
	if len(weather) > 0 {
		w := weather[0]
		ctx.Weather = &WeatherInfo{}
		
		// Get typed weather data or fallback to legacy
		var tempC float64
		var weatherCode int
		if wd := w.GetWeatherData(); wd != nil {
			tempC = wd.TempC
			weatherCode = wd.WeatherCode
			if wd.RainForecast != "" {
				ctx.Weather.RainWarning = wd.RainForecast
				rainForecast = wd.RainForecast
			}
		} else {
			// Legacy: parse temp from name like "☀️ -3°C"
			if w.Name != "" {
				var parsed int
				if _, err := fmt.Sscanf(w.Name, "%*s %d°C", &parsed); err == nil {
					tempC = float64(parsed)
				}
			}
		}
		
		// Get temp and build condition string (avoid "-0" display)
		tempInt := int(math.Round(tempC))
		if tempInt == 0 {
			tempInt = 0 // Ensure no -0
		}
		ctx.Weather.Temp = tempInt
		
		// Build condition from weather code + temp
		icon := weatherIcon(weatherCode)
		if icon == "" {
			icon = "🌡️"
		}
		ctx.Weather.Condition = fmt.Sprintf("%s %d°C", icon, tempInt)
		headerParts = append(headerParts, ctx.Weather.Condition)
	}

	// Prayer times
	prayer := db.Query(lat, lon, 50000, EntityPrayer, 1)
	if len(prayer) == 0 {
		go fetchPrayerTimes(lat, lon) // Fetch in background
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
	if busInfo := GetNearestBusArrivals(lat, lon); busInfo != nil {
		ctx.Bus = &BusInfo{
			StopName: busInfo.StopName,
			Distance: busInfo.Distance,
			Arrivals: busInfo.Arrivals,
		}
		// Format for HTML
		var lines []string
		stopLabel := busInfo.StopName
		if busInfo.Distance >= 30 {
			stopLabel = fmt.Sprintf("%s (%dm)", busInfo.StopName, busInfo.Distance)
		}
		if busInfo.IsStale {
			lines = append(lines, fmt.Sprintf("🚏 %s ⏳", stopLabel))
		} else if busInfo.Distance < 30 {
			lines = append(lines, fmt.Sprintf("🚏 At %s", busInfo.StopName))
		} else {
			lines = append(lines, fmt.Sprintf("🚏 %s", stopLabel))
		}
		for _, arr := range busInfo.Arrivals {
			lines = append(lines, "   "+arr)
		}
		htmlParts = append(htmlParts, strings.Join(lines, "\n"))
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
				// Try typed data first, then legacy map
				var tags map[string]string
				if placeData := p.GetPlaceData(); placeData != nil {
					tags = placeData.Tags
				} else if m, ok := p.Data.(map[string]interface{}); ok {
					if tagsRaw, ok := m["tags"].(map[string]interface{}); ok {
						tags = make(map[string]string)
						for k, v := range tagsRaw {
							if s, ok := v.(string); ok {
								tags[k] = s
							}
						}
					}
				}
				if len(tags) > 0 {
					var addr string
					if num := tags["addr:housenumber"]; num != "" {
						addr = num
					}
					if street := tags["addr:street"]; street != "" {
						if addr != "" {
							addr += " "
						}
						addr += street
					}
					place.Address = addr
					place.Postcode = tags["addr:postcode"]
					place.Hours = tags["opening_hours"]
					if phone := tags["phone"]; phone != "" {
						place.Phone = phone
					} else {
						place.Phone = tags["contact:phone"]
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
