package command

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/asim/quadtree"
)

// EntityType defines the kind of spatial entity
type EntityType string

const (
	EntityPlace   EntityType = "place"   // Static locations (cafes, shops, etc)
	EntityVehicle EntityType = "vehicle" // Moving vehicles
	EntityPerson  EntityType = "person"  // People (with consent)
	EntityEvent   EntityType = "event"   // Time-bounded happenings
	EntityZone    EntityType = "zone"    // Areas/regions
	EntitySensor  EntityType = "sensor"  // IoT devices
)

// Entity represents any spatial object in the world
type Entity struct {
	ID        string                 `json:"id"`
	Type      EntityType             `json:"type"`
	Name      string                 `json:"name"`
	Lat       float64                `json:"lat"`
	Lon       float64                `json:"lon"`
	Data      map[string]interface{} `json:"data"` // Type-specific data
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
	ExpiresAt *time.Time             `json:"expires_at,omitempty"` // nil = never expires
}

// PlaceData extracts place-specific data from an entity
func (e *Entity) PlaceData() *PlaceData {
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
	return &PlaceData{
		Category: e.Data["category"].(string),
		Tags:     strTags,
	}
}

// PlaceData holds place-specific fields
type PlaceData struct {
	Category string            // cafe, restaurant, etc
	Tags     map[string]string // OSM tags
}

// SpatialDB is the main spatial database
type SpatialDB struct {
	mu       sync.RWMutex
	tree     *quadtree.QuadTree
	store    quadtree.Store
	entities map[string]*quadtree.Point // id -> point for updates/deletes
}

var (
	spatialDB     *SpatialDB
	spatialDBOnce sync.Once
)

// GetSpatialDB returns the singleton spatial database
func GetSpatialDB() *SpatialDB {
	spatialDBOnce.Do(func() {
		var err error
		spatialDB, err = NewSpatialDB("spatial.json")
		if err != nil {
			spatialDB = newMemorySpatialDB()
		}
	})
	return spatialDB
}

// NewSpatialDB creates a new spatial database with file persistence
func NewSpatialDB(filename string) (*SpatialDB, error) {
	store, err := quadtree.NewFileStore(filename)
	if err != nil {
		return nil, err
	}

	// Create quadtree covering the world
	center := quadtree.NewPoint(0, 0, nil)
	half := quadtree.NewPoint(90, 180, nil)
	boundary := quadtree.NewAABB(center, half)
	tree := quadtree.New(boundary, 0, nil)

	db := &SpatialDB{
		tree:     tree,
		store:    store,
		entities: make(map[string]*quadtree.Point),
	}

	if err := db.loadFromStore(); err != nil {
		fmt.Printf("[spatial] Error loading: %v\n", err)
	}

	return db, nil
}

func newMemorySpatialDB() *SpatialDB {
	center := quadtree.NewPoint(0, 0, nil)
	half := quadtree.NewPoint(90, 180, nil)
	boundary := quadtree.NewAABB(center, half)
	tree := quadtree.New(boundary, 0, nil)

	return &SpatialDB{
		tree:     tree,
		store:    quadtree.NewMemoryStore(),
		entities: make(map[string]*quadtree.Point),
	}
}

func (db *SpatialDB) loadFromStore() error {
	points, err := db.store.List()
	if err != nil {
		return err
	}

	for id, point := range points {
		data := point.Data()
		if m, ok := data.(map[string]interface{}); ok {
			b, _ := json.Marshal(m)
			var entity Entity
			if err := json.Unmarshal(b, &entity); err == nil {
				// Skip expired entities
				if entity.ExpiresAt != nil && time.Now().After(*entity.ExpiresAt) {
					continue
				}
				newPoint := quadtree.NewPoint(entity.Lat, entity.Lon, &entity)
				if db.tree.Insert(newPoint) {
					db.entities[id] = newPoint
				}
			}
		} else if _, ok := data.(*Entity); ok {
			db.tree.Insert(point)
			db.entities[id] = point
		}
	}

	return nil
}

// GenerateID creates a unique ID for an entity
func GenerateID(entityType EntityType, lat, lon float64, name string) string {
	data := fmt.Sprintf("%s:%.6f:%.6f:%s", entityType, lat, lon, name)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash[:8])
}

// Insert adds or updates an entity in the database
func (db *SpatialDB) Insert(entity *Entity) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if entity.ID == "" {
		entity.ID = GenerateID(entity.Type, entity.Lat, entity.Lon, entity.Name)
	}

	now := time.Now()
	if entity.CreatedAt.IsZero() {
		entity.CreatedAt = now
	}
	entity.UpdatedAt = now

	// Remove existing if updating
	if existing, ok := db.entities[entity.ID]; ok {
		db.tree.Remove(existing)
	}

	point := quadtree.NewPoint(entity.Lat, entity.Lon, entity)
	if !db.tree.Insert(point) {
		return fmt.Errorf("failed to insert entity")
	}

	db.entities[entity.ID] = point
	return db.store.Save(entity.ID, point)
}

// Update moves an entity to a new location
func (db *SpatialDB) Update(id string, lat, lon float64) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	point, ok := db.entities[id]
	if !ok {
		return fmt.Errorf("entity not found")
	}

	entity, ok := point.Data().(*Entity)
	if !ok {
		return fmt.Errorf("invalid entity data")
	}

	// Update via quadtree
	newPoint := quadtree.NewPoint(lat, lon, nil)
	if !db.tree.Update(point, newPoint) {
		return fmt.Errorf("failed to update location")
	}

	entity.Lat = lat
	entity.Lon = lon
	entity.UpdatedAt = time.Now()

	return db.store.Save(id, point)
}

// Delete removes an entity
func (db *SpatialDB) Delete(id string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	point, ok := db.entities[id]
	if !ok {
		return nil
	}

	db.tree.Remove(point)
	delete(db.entities, id)
	return db.store.Delete(id)
}

// Query finds entities near a location
func (db *SpatialDB) Query(lat, lon, radiusMeters float64, entityType EntityType, limit int) []*Entity {
	db.mu.RLock()
	defer db.mu.RUnlock()

	center := quadtree.NewPoint(lat, lon, nil)
	half := center.HalfPoint(radiusMeters)
	boundary := quadtree.NewAABB(center, half)

	now := time.Now()

	filter := func(p *quadtree.Point) bool {
		entity, ok := p.Data().(*Entity)
		if !ok {
			return false
		}

		// Check expiry
		if entity.ExpiresAt != nil && now.After(*entity.ExpiresAt) {
			return false
		}

		// Match type if specified
		if entityType != "" && entity.Type != entityType {
			return false
		}

		return true
	}

	points := db.tree.KNearest(boundary, limit, filter)

	var results []*Entity
	for _, p := range points {
		if entity, ok := p.Data().(*Entity); ok {
			results = append(results, entity)
		}
	}

	return results
}

// QueryPlaces is a convenience method for finding places by category
func (db *SpatialDB) QueryPlaces(lat, lon, radiusMeters float64, category string, limit int) []*Entity {
	db.mu.RLock()
	defer db.mu.RUnlock()

	center := quadtree.NewPoint(lat, lon, nil)
	half := center.HalfPoint(radiusMeters)
	boundary := quadtree.NewAABB(center, half)

	filter := func(p *quadtree.Point) bool {
		entity, ok := p.Data().(*Entity)
		if !ok {
			return false
		}

		if entity.Type != EntityPlace {
			return false
		}

		// Check expiry
		if entity.ExpiresAt != nil && time.Now().After(*entity.ExpiresAt) {
			return false
		}

		// Match category if specified
		if category != "" {
			cat, _ := entity.Data["category"].(string)
			if cat != category {
				return false
			}
		}

		return true
	}

	points := db.tree.KNearest(boundary, limit, filter)

	var results []*Entity
	for _, p := range points {
		if entity, ok := p.Data().(*Entity); ok {
			results = append(results, entity)
		}
	}

	return results
}

// FindByName searches for entities by name near a location
func (db *SpatialDB) FindByName(lat, lon, radiusMeters float64, name string, limit int) []*Entity {
	db.mu.RLock()
	defer db.mu.RUnlock()

	center := quadtree.NewPoint(lat, lon, nil)
	half := center.HalfPoint(radiusMeters)
	boundary := quadtree.NewAABB(center, half)

	nameLower := strings.ToLower(name)

	filter := func(p *quadtree.Point) bool {
		entity, ok := p.Data().(*Entity)
		if !ok {
			return false
		}
		// Check expiry
		if entity.ExpiresAt != nil && time.Now().After(*entity.ExpiresAt) {
			return false
		}
		// Match name (case-insensitive, partial match)
		return strings.Contains(strings.ToLower(entity.Name), nameLower)
	}

	points := db.tree.KNearest(boundary, limit, filter)

	var results []*Entity
	for _, p := range points {
		if entity, ok := p.Data().(*Entity); ok {
			results = append(results, entity)
		}
	}

	return results
}

// Close closes the database
func (db *SpatialDB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	return db.store.Close()
}
