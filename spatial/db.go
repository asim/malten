package spatial

import (
	"encoding/json"
	"fmt"
	"log"
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
			log.Printf("[db] New() failed: %v, using memory store", err)
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

	log.Printf("[db] Starting load from store")
	if err := d.loadFromStore(); err != nil {
		log.Printf("[db] Error loading: %v", err)
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

	log.Printf("[db] Loading %d points from store", len(points))
	var loaded, skipped, failed int
	
	for id, point := range points {
		data := point.Data()
		if m, ok := data.(map[string]interface{}); ok {
			b, _ := json.Marshal(m)
			var entity Entity
			if err := json.Unmarshal(b, &entity); err == nil {
				if entity.ExpiresAt != nil && time.Now().After(*entity.ExpiresAt) {
					skipped++
					continue
				}
				newPoint := quadtree.NewPoint(entity.Lat, entity.Lon, &entity)
				if d.tree.Insert(newPoint) {
					d.entities[id] = newPoint
					loaded++
				} else {
					failed++
				}
			}
		} else if _, ok := data.(*Entity); ok {
			d.tree.Insert(point)
			d.entities[id] = point
			loaded++
		}
	}
	
	log.Printf("[db] Loaded %d entities, skipped %d expired, %d failed to insert", loaded, skipped, failed)
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
		removed := d.tree.Remove(existing)
		if entity.Type == EntityArrival {
			log.Printf("[db] Updating arrival %s: removed=%v", entity.Name, removed)
		}
	}

	point := quadtree.NewPoint(entity.Lat, entity.Lon, entity)
	if !d.tree.Insert(point) {
		log.Printf("[db] FAILED to insert %s %s at (%.4f, %.4f)", entity.Type, entity.Name, entity.Lat, entity.Lon)
		return fmt.Errorf("failed to insert into quadtree")
	}
	if entity.Type == EntityArrival {
		log.Printf("[db] Inserted arrival %s at (%.4f, %.4f)", entity.Name, entity.Lat, entity.Lon)
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
	return d.QueryWithMaxAge(lat, lon, radiusMeters, entityType, limit, 0)
}

// QueryWithMaxAge is like Query but accepts stale data up to maxAge seconds old
// Use maxAge=0 for no stale tolerance (strict expiry)
func (d *DB) QueryWithMaxAge(lat, lon, radiusMeters float64, entityType EntityType, limit int, maxAgeSecs int) []*Entity {
	d.mu.RLock()
	defer d.mu.RUnlock()

	center := quadtree.NewPoint(lat, lon, nil)
	half := center.HalfPoint(radiusMeters)
	boundary := quadtree.NewAABB(center, half)

	now := time.Now()
	filter := func(p *quadtree.Point) bool {
		entity, ok := p.Data().(*Entity)
		if !ok {
			return false
		}
		if entity.ExpiresAt != nil {
			expiry := *entity.ExpiresAt
			if maxAgeSecs > 0 {
				// Allow stale data up to maxAge seconds past expiry
				expiry = expiry.Add(time.Duration(maxAgeSecs) * time.Second)
			}
			if now.After(expiry) {
				return false
			}
		}
		return entity.Type == entityType
	}

	points := d.tree.KNearest(boundary, limit, filter)
	
	// Debug: log when querying arrivals and finding none
	if entityType == EntityArrival && len(points) == 0 {
		// Count total arrivals in entities map
		arrivalCount := 0
		for _, p := range d.entities {
			if e, ok := p.Data().(*Entity); ok && e.Type == EntityArrival {
				arrivalCount++
			}
		}
		log.Printf("[db] QueryWithMaxAge arrivals at %.4f,%.4f r=%.0fm: 0 found (total arrivals in DB: %d)", lat, lon, radiusMeters, arrivalCount)
	}

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

// Delete removes an entity by ID
func (d *DB) Delete(id string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	
	if point, ok := d.entities[id]; ok {
		d.tree.Remove(point)
		delete(d.entities, id)
		d.store.Delete(id)
		
		// Log event
		if d.eventLog != nil {
			d.eventLog.Log("entity.deleted", id, nil)
		}
		return true
	}
	return false
}

// ExtendArrivalsTTL extends the expiry of arrivals near a location
// Used when API returns empty/error to preserve existing data
func (d *DB) ExtendArrivalsTTL(lat, lon, radiusMeters float64) int {
	arrivals := d.Query(lat, lon, radiusMeters, EntityArrival, 10)
	extended := 0
	for _, arr := range arrivals {
		if arr.ExpiresAt != nil {
			newExpiry := time.Now().Add(5 * time.Minute)
			arr.ExpiresAt = &newExpiry
			d.Insert(arr) // Save the updated expiry
			extended++
		}
	}
	if extended > 0 {
		log.Printf("[db] Extended TTL for %d arrivals near %.4f,%.4f", extended, lat, lon)
	}
	return extended
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

// QueryByNameContains searches for entities whose name contains the query string
func (d *DB) QueryByNameContains(lat, lon, radiusMeters float64, nameContains string) []*Entity {
	d.mu.RLock()
	defer d.mu.RUnlock()

	center := quadtree.NewPoint(lat, lon, nil)
	half := center.HalfPoint(radiusMeters)
	boundary := quadtree.NewAABB(center, half)

	nameContains = strings.ToLower(nameContains)
	filter := func(p *quadtree.Point) bool {
		entity, ok := p.Data().(*Entity)
		if !ok {
			return false
		}
		return strings.Contains(strings.ToLower(entity.Name), nameContains)
	}

	points := d.tree.KNearest(boundary, 10, filter)

	var result []*Entity
	for _, p := range points {
		if entity, ok := p.Data().(*Entity); ok {
			result = append(result, entity)
		}
	}
	return result
}

// DBStats holds statistics about the database
type DBStats struct {
	Total    int
	Agents   int
	Weather  int
	Prayer   int
	Arrivals int
	Places   int
}

// Stats returns statistics about entities in the database
func (d *DB) Stats() DBStats {
	d.mu.RLock()
	defer d.mu.RUnlock()

	stats := DBStats{}
	for _, point := range d.entities {
		entity, ok := point.Data().(*Entity)
		if !ok {
			continue
		}
		stats.Total++
		switch entity.Type {
		case EntityAgent:
			stats.Agents++
		case EntityWeather:
			stats.Weather++
		case EntityPrayer:
			stats.Prayer++
		case EntityArrival:
			stats.Arrivals++
		case EntityPlace:
			stats.Places++
		}
	}
	return stats
}

// CountByAgentID counts entities indexed by a specific agent
func (d *DB) CountByAgentID(agentID string, entityType EntityType) int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	count := 0
	for _, point := range d.entities {
		entity, ok := point.Data().(*Entity)
		if !ok {
			continue
		}
		if entity.Type != entityType {
			continue
		}
		if aid, ok := entity.Data["agent_id"].(string); ok && aid == agentID {
			count++
		}
	}
	return count
}

// CleanupExpired removes all expired entities from the database
func (d *DB) CleanupExpired() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	var toDelete []string

	for id, point := range d.entities {
		entity, ok := point.Data().(*Entity)
		if !ok {
			continue
		}
		if entity.ExpiresAt != nil && now.After(*entity.ExpiresAt) {
			toDelete = append(toDelete, id)
		}
	}

	for _, id := range toDelete {
		if point, ok := d.entities[id]; ok {
			d.tree.Remove(point)
			delete(d.entities, id)
			d.store.Delete(id)
		}
	}

	if len(toDelete) > 0 {
		log.Printf("[db] Cleaned up %d expired entities", len(toDelete))
	}

	return len(toDelete)
}

// CleanupDuplicateArrivals removes duplicate arrival entries for the same stop
func (d *DB) CleanupDuplicateArrivals() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Group arrivals by stop_id
	byStop := make(map[string][]*Entity)
	for _, point := range d.entities {
		entity, ok := point.Data().(*Entity)
		if !ok || entity.Type != EntityArrival {
			continue
		}
		stopID, _ := entity.Data["stop_id"].(string)
		if stopID == "" {
			continue
		}
		byStop[stopID] = append(byStop[stopID], entity)
	}

	var toDelete []string
	for _, entities := range byStop {
		if len(entities) <= 1 {
			continue
		}
		// Keep the newest, delete the rest
		var newest *Entity
		for _, e := range entities {
			if newest == nil || e.UpdatedAt.After(newest.UpdatedAt) {
				newest = e
			}
		}
		for _, e := range entities {
			if e.ID != newest.ID {
				toDelete = append(toDelete, e.ID)
			}
		}
	}

	for _, id := range toDelete {
		if point, ok := d.entities[id]; ok {
			d.tree.Remove(point)
			delete(d.entities, id)
			d.store.Delete(id)
		}
	}

	if len(toDelete) > 0 {
		log.Printf("[db] Cleaned up %d duplicate arrivals", len(toDelete))
	}

	return len(toDelete)
}

// CleanupStore removes expired and duplicate entries from the persistent store
func (d *DB) CleanupStore() (expired int, duplicates int, err error) {
	points, err := d.store.List()
	if err != nil {
		return 0, 0, err
	}

	now := time.Now()
	var toDelete []string
	byStopID := make(map[string][]struct{ id string; updated time.Time })

	for id, point := range points {
		data := point.Data()
		m, ok := data.(map[string]interface{})
		if !ok {
			continue
		}

		b, _ := json.Marshal(m)
		var entity Entity
		if err := json.Unmarshal(b, &entity); err != nil {
			continue
		}

		// Check if expired
		if entity.ExpiresAt != nil && now.After(*entity.ExpiresAt) {
			toDelete = append(toDelete, id)
			expired++
			continue
		}

		// Track arrivals by stop_id for duplicate detection
		if entity.Type == EntityArrival {
			stopID, _ := entity.Data["stop_id"].(string)
			if stopID != "" {
				byStopID[stopID] = append(byStopID[stopID], struct{ id string; updated time.Time }{id, entity.UpdatedAt})
			}
		}
	}

	// Find duplicate arrivals (keep newest per stop)
	for _, entries := range byStopID {
		if len(entries) <= 1 {
			continue
		}
		// Find newest
		var newestID string
		var newestTime time.Time
		for _, e := range entries {
			if e.updated.After(newestTime) {
				newestTime = e.updated
				newestID = e.id
			}
		}
		// Mark others for deletion
		for _, e := range entries {
			if e.id != newestID {
				toDelete = append(toDelete, e.id)
				duplicates++
			}
		}
	}

	// Delete from store
	for _, id := range toDelete {
		d.store.Delete(id)
	}

	log.Printf("[db] Store cleanup: removed %d expired, %d duplicates", expired, duplicates)
	return expired, duplicates, nil
}

// StartBackgroundCleanup starts a goroutine that periodically cleans expired arrivals
func (d *DB) StartBackgroundCleanup(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		
		for range ticker.C {
			d.cleanupExpiredArrivals()
		}
	}()
}

// cleanupExpiredArrivals removes only expired arrival entities
func (d *DB) cleanupExpiredArrivals() {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	var toDelete []string

	for id, point := range d.entities {
		entity, ok := point.Data().(*Entity)
		if !ok {
			continue
		}
		// Only clean arrivals - other entities (places, agents) don't expire
		if entity.Type != EntityArrival {
			continue
		}
		if entity.ExpiresAt != nil && now.After(*entity.ExpiresAt) {
			toDelete = append(toDelete, id)
		}
	}

	for _, id := range toDelete {
		if point, ok := d.entities[id]; ok {
			d.tree.Remove(point)
			delete(d.entities, id)
			d.store.Delete(id)
		}
	}

	if len(toDelete) > 0 {
		log.Printf("[db] Background cleanup: removed %d expired arrivals", len(toDelete))
	}
}
