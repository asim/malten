package spatial

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"
)

// EntityType defines the kind of spatial entity
type EntityType string

const (
	EntityPlace      EntityType = "place"      // Static locations (cafes, shops, etc)
	EntityAgent      EntityType = "agent"      // Area indexers
	EntityVehicle    EntityType = "vehicle"    // Moving vehicles (buses, trains)
	EntityPerson     EntityType = "person"     // People (with consent)
	EntityEvent      EntityType = "event"      // Time-bounded happenings
	EntityZone       EntityType = "zone"       // Areas/regions
	EntitySensor     EntityType = "sensor"     // IoT devices
	EntityWeather    EntityType = "weather"    // Weather conditions
	EntityPrayer     EntityType = "prayer"     // Prayer times
	EntityArrival    EntityType = "arrival"    // Transport arrivals
	EntityLocation   EntityType = "location"   // Reverse geocoded location names
	EntityNews       EntityType = "news"       // Breaking news/headlines
	EntityDisruption EntityType = "disruption" // Traffic disruptions
	EntityStreet     EntityType = "street"     // Street/road geometry
)

// EntityData is the interface that all typed entity data must implement
type EntityData interface {
	entityData() // marker method
}

// Entity represents any spatial object in the world
type Entity struct {
	ID        string      `json:"id"`
	Type      EntityType  `json:"type"`
	Name      string      `json:"name"`
	Lat       float64     `json:"lat"`
	Lon       float64     `json:"lon"`
	Data      interface{} `json:"data"` // EntityData or legacy map[string]interface{}
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
	ExpiresAt *time.Time  `json:"expires_at,omitempty"`
}

// GenerateID creates a unique ID for an entity
func GenerateID(entityType EntityType, lat, lon float64, name string) string {
	data := fmt.Sprintf("%s:%.6f:%.6f:%s", entityType, lat, lon, name)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash[:8])
}

// =============================================================================
// Typed Data Structures
// =============================================================================

// ArrivalData holds transport arrival information
type ArrivalData struct {
	StopID   string       `json:"stop_id"`
	StopName string       `json:"stop_name"`
	StopType string       `json:"stop_type"`
	Arrivals []BusArrival `json:"arrivals"`
}

func (ArrivalData) entityData() {}

// BusArrival represents a single bus/train arrival
type BusArrival struct {
	Line        string    `json:"line"`
	Destination string    `json:"destination"`
	ArrivalTime time.Time `json:"arrival_time"`
}

// MinutesUntil calculates minutes until arrival from now
func (b BusArrival) MinutesUntil() int {
	mins := int(time.Until(b.ArrivalTime).Minutes())
	if mins < 0 {
		return 0
	}
	return mins
}

// WeatherData holds weather information
type WeatherData struct {
	TempC        float64 `json:"temp_c"`
	WeatherCode  int     `json:"weather_code"`
	RainForecast string  `json:"rain_forecast"`
}

func (WeatherData) entityData() {}

// PrayerData holds prayer time information
type PrayerData struct {
	Timings map[string]string `json:"timings"`
	Current string            `json:"current"`
	Next    string            `json:"next"`
}

func (PrayerData) entityData() {}

// PlaceData holds place-specific fields
type PlaceData struct {
	Category string            `json:"category"`
	Tags     map[string]string `json:"tags"`
	AgentID  string            `json:"agent_id"`
}

func (PlaceData) entityData() {}

// AgentEntityData holds agent-specific fields
type AgentEntityData struct {
	Radius    float64    `json:"radius"`
	Status    string     `json:"status"`
	POICount  int        `json:"poi_count"`
	LastIndex *time.Time `json:"last_index,omitempty"`
	LastLive  *time.Time `json:"last_live,omitempty"`
	// Exploration state (persisted)
	HomeLat    float64 `json:"home_lat,omitempty"`
	HomeLon    float64 `json:"home_lon,omitempty"`
	TotalSteps int     `json:"total_steps,omitempty"`
	StepsToday int     `json:"steps_today,omitempty"`
}

func (AgentEntityData) entityData() {}

// StreetData holds street geometry
type StreetData struct {
	Points [][]float64 `json:"points"`
	Length float64     `json:"length"`
	ToName string      `json:"to_name"`
}

func (StreetData) entityData() {}

// LocationData holds reverse geocoded location info
type LocationData struct {
	Street   string `json:"street"`
	Postcode string `json:"postcode"`
}

func (LocationData) entityData() {}

// =============================================================================
// Data Access Helpers - return typed data or fallback to legacy map
// =============================================================================

// GetArrivalData returns typed arrival data or nil
func (e *Entity) GetArrivalData() *ArrivalData {
	if e.Type != EntityArrival {
		return nil
	}
	// Try typed data first
	if ad, ok := e.Data.(*ArrivalData); ok {
		return ad
	}
	// Fallback to legacy map
	if m, ok := e.Data.(map[string]interface{}); ok {
		return arrivalDataFromMap(m)
	}
	return nil
}

// GetWeatherData returns typed weather data or nil
func (e *Entity) GetWeatherData() *WeatherData {
	if e.Type != EntityWeather {
		return nil
	}
	if wd, ok := e.Data.(*WeatherData); ok {
		return wd
	}
	if m, ok := e.Data.(map[string]interface{}); ok {
		return weatherDataFromMap(m)
	}
	return nil
}

// GetPrayerData returns typed prayer data or nil
func (e *Entity) GetPrayerData() *PrayerData {
	if e.Type != EntityPrayer {
		return nil
	}
	if pd, ok := e.Data.(*PrayerData); ok {
		return pd
	}
	if m, ok := e.Data.(map[string]interface{}); ok {
		return prayerDataFromMap(m)
	}
	return nil
}

// GetPlaceData returns typed place data or nil
func (e *Entity) GetPlaceData() *PlaceData {
	if e.Type != EntityPlace {
		return nil
	}
	if pd, ok := e.Data.(*PlaceData); ok {
		return pd
	}
	if m, ok := e.Data.(map[string]interface{}); ok {
		return placeDataFromMap(m)
	}
	return nil
}

// GetAgentData returns typed agent data or nil
func (e *Entity) GetAgentData() *AgentEntityData {
	if e.Type != EntityAgent {
		return nil
	}
	if ad, ok := e.Data.(*AgentEntityData); ok {
		return ad
	}
	if m, ok := e.Data.(map[string]interface{}); ok {
		return agentDataFromMap(m)
	}
	return nil
}

// GetStreetData returns typed street data or nil
func (e *Entity) GetStreetData() *StreetData {
	if e.Type != EntityStreet {
		return nil
	}
	if sd, ok := e.Data.(*StreetData); ok {
		return sd
	}
	if m, ok := e.Data.(map[string]interface{}); ok {
		return streetDataFromMap(m)
	}
	return nil
}

// GetLocationData returns typed location data or nil
func (e *Entity) GetLocationData() *LocationData {
	if e.Type != EntityLocation {
		return nil
	}
	if ld, ok := e.Data.(*LocationData); ok {
		return ld
	}
	if m, ok := e.Data.(map[string]interface{}); ok {
		return locationDataFromMap(m)
	}
	return nil
}

// =============================================================================
// Legacy Map Converters - for backward compatibility with existing JSON data
// =============================================================================

func arrivalDataFromMap(m map[string]interface{}) *ArrivalData {
	ad := &ArrivalData{}
	ad.StopID, _ = m["stop_id"].(string)
	ad.StopName, _ = m["stop_name"].(string)
	ad.StopType, _ = m["stop_type"].(string)

	// Parse arrivals array
	if arrData, ok := m["arrivals"].([]interface{}); ok {
		for _, a := range arrData {
			if amap, ok := a.(map[string]interface{}); ok {
				ba := BusArrival{}
				ba.Line, _ = amap["line"].(string)
				ba.Destination, _ = amap["destination"].(string)
				// Try new format (arrival_time) first
				if arrTimeStr, ok := amap["arrival_time"].(string); ok {
					if t, err := time.Parse(time.RFC3339Nano, arrTimeStr); err == nil {
						ba.ArrivalTime = t
					} else if t, err := time.Parse(time.RFC3339, arrTimeStr); err == nil {
						ba.ArrivalTime = t
					}
				} else if minsFloat, ok := amap["minutes"].(float64); ok {
					// Legacy format - convert to arrival time
					ba.ArrivalTime = time.Now().Add(time.Duration(minsFloat) * time.Minute)
				}
				ad.Arrivals = append(ad.Arrivals, ba)
			}
		}
	}
	return ad
}

func weatherDataFromMap(m map[string]interface{}) *WeatherData {
	wd := &WeatherData{}
	if temp, ok := m["temp_c"].(float64); ok {
		wd.TempC = temp
	}
	if code, ok := m["weather_code"].(float64); ok {
		wd.WeatherCode = int(code)
	}
	wd.RainForecast, _ = m["rain_forecast"].(string)
	return wd
}

func prayerDataFromMap(m map[string]interface{}) *PrayerData {
	pd := &PrayerData{}
	pd.Current, _ = m["current"].(string)
	pd.Next, _ = m["next"].(string)

	// Parse timings
	if timingsRaw, ok := m["timings"]; ok {
		pd.Timings = make(map[string]string)
		switch t := timingsRaw.(type) {
		case map[string]interface{}:
			for k, v := range t {
				if s, ok := v.(string); ok {
					pd.Timings[k] = s
				}
			}
		case map[string]string:
			pd.Timings = t
		}
	}
	return pd
}

func placeDataFromMap(m map[string]interface{}) *PlaceData {
	pd := &PlaceData{}
	pd.Category, _ = m["category"].(string)
	pd.AgentID, _ = m["agent_id"].(string)

	if tags, ok := m["tags"].(map[string]interface{}); ok {
		pd.Tags = make(map[string]string)
		for k, v := range tags {
			if s, ok := v.(string); ok {
				pd.Tags[k] = s
			}
		}
	}
	return pd
}

func agentDataFromMap(m map[string]interface{}) *AgentEntityData {
	ad := &AgentEntityData{}
	ad.Radius, _ = m["radius"].(float64)
	ad.Status, _ = m["status"].(string)
	if poiCount, ok := m["poi_count"].(float64); ok {
		ad.POICount = int(poiCount)
	}
	if li, ok := m["last_index"].(string); ok && li != "" {
		if t, err := time.Parse(time.RFC3339, li); err == nil {
			ad.LastIndex = &t
		}
	}
	if ll, ok := m["last_live"].(string); ok && ll != "" {
		if t, err := time.Parse(time.RFC3339, ll); err == nil {
			ad.LastLive = &t
		}
	}
	return ad
}

func streetDataFromMap(m map[string]interface{}) *StreetData {
	sd := &StreetData{}
	sd.ToName, _ = m["to_name"].(string)
	sd.Length, _ = m["length"].(float64)

	if points, ok := m["points"].([]interface{}); ok {
		for _, p := range points {
			if pt, ok := p.([]interface{}); ok && len(pt) >= 2 {
				lon, _ := pt[0].(float64)
				lat, _ := pt[1].(float64)
				sd.Points = append(sd.Points, []float64{lon, lat})
			}
		}
	}
	return sd
}

func locationDataFromMap(m map[string]interface{}) *LocationData {
	ld := &LocationData{}
	ld.Street, _ = m["street"].(string)
	ld.Postcode, _ = m["postcode"].(string)
	return ld
}

// =============================================================================
// JSON Custom Unmarshaling for backward compatibility
// =============================================================================

// entityJSON is used for JSON unmarshaling
type entityJSON struct {
	ID        string          `json:"id"`
	Type      EntityType      `json:"type"`
	Name      string          `json:"name"`
	Lat       float64         `json:"lat"`
	Lon       float64         `json:"lon"`
	Data      json.RawMessage `json:"data"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	ExpiresAt *time.Time      `json:"expires_at,omitempty"`
}

// UnmarshalJSON implements custom unmarshaling for backward compatibility
func (e *Entity) UnmarshalJSON(b []byte) error {
	var raw entityJSON
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}

	e.ID = raw.ID
	e.Type = raw.Type
	e.Name = raw.Name
	e.Lat = raw.Lat
	e.Lon = raw.Lon
	e.CreatedAt = raw.CreatedAt
	e.UpdatedAt = raw.UpdatedAt
	e.ExpiresAt = raw.ExpiresAt

	// Try to unmarshal data into typed struct based on entity type
	// Fall back to map[string]interface{} for unknown types or if typed unmarshal fails
	if len(raw.Data) == 0 || string(raw.Data) == "null" {
		e.Data = nil
		return nil
	}

	switch raw.Type {
	case EntityArrival:
		var ad ArrivalData
		if err := json.Unmarshal(raw.Data, &ad); err == nil {
			e.Data = &ad
			return nil
		}
	case EntityWeather:
		var wd WeatherData
		if err := json.Unmarshal(raw.Data, &wd); err == nil {
			e.Data = &wd
			return nil
		}
	case EntityPrayer:
		var pd PrayerData
		if err := json.Unmarshal(raw.Data, &pd); err == nil {
			e.Data = &pd
			return nil
		}
	case EntityPlace:
		var pd PlaceData
		if err := json.Unmarshal(raw.Data, &pd); err == nil {
			e.Data = &pd
			return nil
		}
	case EntityAgent:
		var ad AgentEntityData
		if err := json.Unmarshal(raw.Data, &ad); err == nil {
			e.Data = &ad
			return nil
		}
	case EntityStreet:
		var sd StreetData
		if err := json.Unmarshal(raw.Data, &sd); err == nil {
			e.Data = &sd
			return nil
		}
	case EntityLocation:
		var ld LocationData
		if err := json.Unmarshal(raw.Data, &ld); err == nil {
			e.Data = &ld
			return nil
		}
	}

	// Fallback: unmarshal as generic map
	var m map[string]interface{}
	if err := json.Unmarshal(raw.Data, &m); err != nil {
		return err
	}
	e.Data = m
	return nil
}
