package spatial

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/asim/quadtree"
)

// DB is the spatial database
type DB struct {
	mu       sync.RWMutex
	tree     *quadtree.QuadTree
	store    quadtree.Store
	entities map[string]*quadtree.Point
	eventLog *EventLog
}

var (
	db     *DB
	dbOnce sync.Once
)

// Get returns the singleton spatial database
func Get() *DB {
	dbOnce.Do(func() {
		var err error
		db, err = New("spatial.json", "events.jsonl")
		if err != nil {
			db = newMemory()
		}
		// Start agent loops - they handle live data
		db.recoverStaleAgents()
	})
	return db
}

// New creates a new spatial database with file persistence
func New(spatialFile, eventFile string) (*DB, error) {
	store, err := quadtree.NewFileStore(spatialFile)
	if err != nil {
		return nil, err
	}

	eventLog, err := NewEventLog(eventFile)
	if err != nil {
		return nil, err
	}

	center := quadtree.NewPoint(0, 0, nil)
	half := quadtree.NewPoint(90, 180, nil)
	boundary := quadtree.NewAABB(center, half)
	tree := quadtree.New(boundary, 0, nil)

	d := &DB{
		tree:     tree,
		store:    store,
		entities: make(map[string]*quadtree.Point),
		eventLog: eventLog,
	}

	if err := d.loadFromStore(); err != nil {
		fmt.Printf("[spatial] Error loading: %v\n", err)
	}

	return d, nil
}

func newMemory() *DB {
	center := quadtree.NewPoint(0, 0, nil)
	half := quadtree.NewPoint(90, 180, nil)
	boundary := quadtree.NewAABB(center, half)
	tree := quadtree.New(boundary, 0, nil)

	return &DB{
		tree:     tree,
		store:    quadtree.NewMemoryStore(),
		entities: make(map[string]*quadtree.Point),
		eventLog: nil,
	}
}

func (d *DB) loadFromStore() error {
	points, err := d.store.List()
	if err != nil {
		return err
	}

	for id, point := range points {
		data := point.Data()
		if m, ok := data.(map[string]interface{}); ok {
			b, _ := json.Marshal(m)
			var entity Entity
			if err := json.Unmarshal(b, &entity); err == nil {
				if entity.ExpiresAt != nil && time.Now().After(*entity.ExpiresAt) {
					continue
				}
				newPoint := quadtree.NewPoint(entity.Lat, entity.Lon, &entity)
				if d.tree.Insert(newPoint) {
					d.entities[id] = newPoint
				}
			}
		} else if _, ok := data.(*Entity); ok {
			d.tree.Insert(point)
			d.entities[id] = point
		}
	}

	return nil
}

// Insert adds or updates an entity
func (d *DB) Insert(entity *Entity) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	isNew := entity.ID == ""
	if isNew {
		entity.ID = GenerateID(entity.Type, entity.Lat, entity.Lon, entity.Name)
	}

	now := time.Now()
	if entity.CreatedAt.IsZero() {
		entity.CreatedAt = now
	}
	entity.UpdatedAt = now

	// Remove existing if updating
	if existing, ok := d.entities[entity.ID]; ok {
		d.tree.Remove(existing)
	}

	point := quadtree.NewPoint(entity.Lat, entity.Lon, entity)
	if !d.tree.Insert(point) {
		return fmt.Errorf("failed to insert into quadtree")
	}

	d.entities[entity.ID] = point

	if err := d.store.Save(entity.ID, point); err != nil {
		return err
	}

	// Log event
	if d.eventLog != nil {
		eventType := "entity.updated"
		if isNew {
			eventType = "entity.created"
		}
		d.eventLog.Log(eventType, entity.ID, map[string]interface{}{
			"type": entity.Type,
			"name": entity.Name,
			"lat":  entity.Lat,
			"lon":  entity.Lon,
		})
	}

	return nil
}

// Query finds entities near a location
func (d *DB) Query(lat, lon, radiusMeters float64, entityType EntityType, limit int) []*Entity {
	d.mu.RLock()
	defer d.mu.RUnlock()

	center := quadtree.NewPoint(lat, lon, nil)
	half := center.HalfPoint(radiusMeters)
	boundary := quadtree.NewAABB(center, half)

	filter := func(p *quadtree.Point) bool {
		entity, ok := p.Data().(*Entity)
		if !ok {
			return false
		}
		if entity.ExpiresAt != nil && time.Now().After(*entity.ExpiresAt) {
			return false
		}
		return entity.Type == entityType
	}

	points := d.tree.KNearest(boundary, limit, filter)

	var results []*Entity
	for _, p := range points {
		if entity, ok := p.Data().(*Entity); ok {
			results = append(results, entity)
		}
	}
	return results
}

// QueryPlaces finds places by category
func (d *DB) QueryPlaces(lat, lon, radiusMeters float64, category string, limit int) []*Entity {
	d.mu.RLock()
	defer d.mu.RUnlock()

	center := quadtree.NewPoint(lat, lon, nil)
	half := center.HalfPoint(radiusMeters)
	boundary := quadtree.NewAABB(center, half)

	filter := func(p *quadtree.Point) bool {
		entity, ok := p.Data().(*Entity)
		if !ok || entity.Type != EntityPlace {
			return false
		}
		if entity.ExpiresAt != nil && time.Now().After(*entity.ExpiresAt) {
			return false
		}
		cat, _ := entity.Data["category"].(string)
		return cat == category
	}

	points := d.tree.KNearest(boundary, limit, filter)

	var results []*Entity
	for _, p := range points {
		if entity, ok := p.Data().(*Entity); ok {
			results = append(results, entity)
		}
	}
	return results
}

// FindByName searches entities by name
func (d *DB) FindByName(lat, lon, radiusMeters float64, name string, limit int) []*Entity {
	d.mu.RLock()
	defer d.mu.RUnlock()

	center := quadtree.NewPoint(lat, lon, nil)
	half := center.HalfPoint(radiusMeters)
	boundary := quadtree.NewAABB(center, half)

	nameLower := strings.ToLower(name)

	filter := func(p *quadtree.Point) bool {
		entity, ok := p.Data().(*Entity)
		if !ok || entity.Type != EntityPlace {
			return false
		}
		if entity.ExpiresAt != nil && time.Now().After(*entity.ExpiresAt) {
			return false
		}
		return strings.Contains(strings.ToLower(entity.Name), nameLower)
	}

	points := d.tree.KNearest(boundary, limit, filter)

	var results []*Entity
	for _, p := range points {
		if entity, ok := p.Data().(*Entity); ok {
			results = append(results, entity)
		}
	}
	return results
}

// GetByID retrieves an entity by its ID
func (d *DB) GetByID(id string) *Entity {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	if point, ok := d.entities[id]; ok {
		if entity, ok := point.Data().(*Entity); ok {
			if entity.ExpiresAt != nil && time.Now().After(*entity.ExpiresAt) {
				return nil
			}
			return entity
		}
	}
	return nil
}

// Close closes the database
func (d *DB) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.eventLog != nil {
		d.eventLog.Close()
	}
	return d.store.Close()
}
