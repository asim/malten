package spatial

import (
	"fmt"
	"sync"
)

// AreaLocks prevents race conditions when multiple agents
// try to update data for overlapping geographic areas.
//
// The key insight: agents at similar locations will try to fetch
// the same data (weather, transport, etc). We need to serialize
// these operations per-area, not per-agent.
var (
	areaLocks   = make(map[string]*sync.Mutex)
	areaLocksMu sync.Mutex
)

// AreaLockPrecision controls the grid size for area locks.
// 2 decimal places = ~1km grid
// 3 decimal places = ~100m grid
const AreaLockPrecision = 2

// GetAreaLock returns a mutex for the given geographic area.
// Areas are rounded to a grid to group nearby operations.
func GetAreaLock(lat, lon float64) *sync.Mutex {
	// Round to grid precision
	key := fmt.Sprintf("%.2f,%.2f", lat, lon)
	
	areaLocksMu.Lock()
	defer areaLocksMu.Unlock()
	
	if lock, ok := areaLocks[key]; ok {
		return lock
	}
	lock := &sync.Mutex{}
	areaLocks[key] = lock
	return lock
}

// WithAreaLock executes fn while holding the area lock.
// Use this for any operation that checks cache then fetches.
func WithAreaLock(lat, lon float64, fn func()) {
	lock := GetAreaLock(lat, lon)
	lock.Lock()
	defer lock.Unlock()
	fn()
}
