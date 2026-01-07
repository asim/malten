package data

import (
	"sync"
	"time"
)

// SessionsState tracks in-memory session data (not persisted)
type SessionsState struct {
	mu        sync.RWMutex
	Locations map[string]*SessionLocation
}

// SessionLocation is a session's last known location
type SessionLocation struct {
	SessionID string
	Lat       float64
	Lon       float64
	UpdatedAt time.Time
}

// GetLocation returns a session's last known location
func (s *SessionsState) GetLocation(sessionID string) (lat, lon float64, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	loc, exists := s.Locations[sessionID]
	if !exists {
		return 0, 0, false
	}
	return loc.Lat, loc.Lon, true
}

// SetLocation updates a session's location
func (s *SessionsState) SetLocation(sessionID string, lat, lon float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Locations[sessionID] = &SessionLocation{
		SessionID: sessionID,
		Lat:       lat,
		Lon:       lon,
		UpdatedAt: time.Now(),
	}
}
