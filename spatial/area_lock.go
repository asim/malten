package spatial

import (
	"fmt"
	"sync"
)

// AreaLocks prevents race conditions when multiple agents
// try to update data for overlapping geographic areas.
//
// The lock covers: cache check → API fetch → insert
// This means concurrent fetches for the same area will serialize,
// but that's acceptable for background agent updates.

var (
	areaLocks   = make(map[string]*sync.Mutex)
	areaLocksMu sync.Mutex
)

// AreaLockPrecision controls the grid size for area locks.
// 2 decimal places = ~1km grid
const AreaLockPrecision = 2

// GetAreaLock returns a mutex for the given geographic area.
// Areas are rounded to a grid to group nearby operations.
func GetAreaLock(lat, lon float64) *sync.Mutex {
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
