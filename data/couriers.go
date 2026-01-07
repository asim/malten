package data

import (
	"path/filepath"
	"sync"
	"time"
)

// CouriersFile manages courier_state.json and regional_couriers.json
type CouriersFile struct {
	mu       sync.RWMutex
	Couriers map[string]*CourierInfo `json:"couriers"`
	Regional *RegionalConfig         `json:"regional"`
}

// CourierInfo tracks a location's courier state
type CourierInfo struct {
	ID           string    `json:"id"`
	Lat          float64   `json:"lat"`
	Lon          float64   `json:"lon"`
	LastRun      time.Time `json:"last_run"`
	NextRun      time.Time `json:"next_run"`
	IntervalMins int       `json:"interval_mins"`
	Enabled      bool      `json:"enabled"`
}

// RegionalConfig stores regional courier settings
type RegionalConfig struct {
	Enabled bool                       `json:"enabled"`
	Regions map[string]*RegionSettings `json:"regions"`
}

// RegionSettings for a single region
type RegionSettings struct {
	Name        string    `json:"name"`
	Lat         float64   `json:"lat"`
	Lon         float64   `json:"lon"`
	RadiusKm    float64   `json:"radius_km"`
	LastRun     time.Time `json:"last_run"`
	IntervalMin int       `json:"interval_min"`
}

// Load reads courier state files
func (c *CouriersFile) Load() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := loadJSON(filepath.Join(dataDir, "courier_state.json"), c); err != nil {
		return err
	}

	var regional RegionalConfig
	if err := loadJSON(filepath.Join(dataDir, "regional_couriers.json"), &regional); err == nil {
		c.Regional = &regional
	}

	return nil
}

// Save writes courier state files
func (c *CouriersFile) Save() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if err := saveJSON(filepath.Join(dataDir, "courier_state.json"), c); err != nil {
		return err
	}

	if c.Regional != nil {
		if err := saveJSON(filepath.Join(dataDir, "regional_couriers.json"), c.Regional); err != nil {
			return err
		}
	}

	return nil
}

// GetCourier returns a courier by ID
func (c *CouriersFile) GetCourier(id string) *CourierInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Couriers[id]
}

// SetCourier adds or updates a courier
func (c *CouriersFile) SetCourier(courier *CourierInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Couriers[courier.ID] = courier
}
