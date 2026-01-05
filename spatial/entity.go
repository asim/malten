package spatial

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// EntityType defines the kind of spatial entity
type EntityType string

const (
	EntityPlace    EntityType = "place"    // Static locations (cafes, shops, etc)
	EntityAgent    EntityType = "agent"    // Area indexers
	EntityVehicle  EntityType = "vehicle"  // Moving vehicles (buses, trains)
	EntityPerson   EntityType = "person"   // People (with consent)
	EntityEvent    EntityType = "event"    // Time-bounded happenings
	EntityZone     EntityType = "zone"     // Areas/regions
	EntitySensor   EntityType = "sensor"   // IoT devices
	EntityWeather  EntityType = "weather"  // Weather conditions
	EntityPrayer   EntityType = "prayer"   // Prayer times
	EntityArrival  EntityType = "arrival"  // Transport arrivals
	EntityLocation EntityType = "location" // Reverse geocoded location names
	EntityNews       EntityType = "news"       // Breaking news/headlines
	EntityDisruption EntityType = "disruption" // Traffic disruptions
	EntityStreet     EntityType = "street"     // Street/road geometry
)

// Entity represents any spatial object in the world
type Entity struct {
	ID        string                 `json:"id"`
	Type      EntityType             `json:"type"`
	Name      string                 `json:"name"`
	Lat       float64                `json:"lat"`
	Lon       float64                `json:"lon"`
	Data      map[string]interface{} `json:"data"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
	ExpiresAt *time.Time             `json:"expires_at,omitempty"`
}

// GenerateID creates a unique ID for an entity
func GenerateID(entityType EntityType, lat, lon float64, name string) string {
	data := fmt.Sprintf("%s:%.6f:%.6f:%s", entityType, lat, lon, name)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash[:8])
}

// PlaceData holds place-specific fields
type PlaceData struct {
	Category string
	Tags     map[string]string
	AgentID  string
}

// GetPlaceData extracts place-specific data from an entity
func (e *Entity) GetPlaceData() *PlaceData {
	if e.Type != EntityPlace {
		return nil
	}
	tags, _ := e.Data["tags"].(map[string]interface{})
	strTags := make(map[string]string)
	for k, v := range tags {
		if s, ok := v.(string); ok {
			strTags[k] = s
		}
	}
	cat, _ := e.Data["category"].(string)
	agentID, _ := e.Data["agent_id"].(string)
	return &PlaceData{
		Category: cat,
		Tags:     strTags,
		AgentID:  agentID,
	}
}

// AgentData holds agent-specific fields
type AgentData struct {
	Radius    float64
	Status    string
	POICount  int
	LastIndex *time.Time
}

// GetAgentData extracts agent-specific data from an entity
func (e *Entity) GetAgentData() *AgentData {
	if e.Type != EntityAgent {
		return nil
	}
	radius, _ := e.Data["radius"].(float64)
	status, _ := e.Data["status"].(string)
	poiCount, _ := e.Data["poi_count"].(float64)
	var lastIndex *time.Time
	if li, ok := e.Data["last_index"].(string); ok && li != "" {
		if t, err := time.Parse(time.RFC3339, li); err == nil {
			lastIndex = &t
		}
	}
	return &AgentData{
		Radius:    radius,
		Status:    status,
		POICount:  int(poiCount),
		LastIndex: lastIndex,
	}
}
