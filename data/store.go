// Package data provides unified state management for Malten.
// Each data file has explicit Load/Save methods.
package data

import (
	"encoding/json"
	"log"
	"os"
	"sync"
	"time"
)

var dataDir = "."

// SetDataDir sets the directory for all data files
func SetDataDir(dir string) {
	dataDir = dir
}

// DataDir returns the current data directory
func DataDir() string {
	return dataDir
}

//
// Subscriptions - push notification subscriptions
//

var (
	subscriptions     *SubscriptionsFile
	subscriptionsOnce sync.Once
)

func Subscriptions() *SubscriptionsFile {
	subscriptionsOnce.Do(func() {
		subscriptions = &SubscriptionsFile{
			Users: make(map[string]*PushUser),
		}
	})
	return subscriptions
}

//
// Notifications - dedupe history and content keys
//

var (
	notifications     *NotificationsFile
	notificationsOnce sync.Once
)

func Notifications() *NotificationsFile {
	notificationsOnce.Do(func() {
		notifications = &NotificationsFile{
			History:     make(map[string][]HistoryItem),
			ContentKeys: make(map[string]map[string]int64),
		}
	})
	return notifications
}

//
// Couriers - courier scheduling state
//

var (
	couriers     *CouriersFile
	couriersOnce sync.Once
)

func Couriers() *CouriersFile {
	couriersOnce.Do(func() {
		couriers = &CouriersFile{
			Couriers: make(map[string]*CourierInfo),
			Regional: &RegionalConfig{
				Enabled: true,
				Regions: make(map[string]*RegionSettings),
			},
		}
	})
	return couriers
}

//
// Sessions - in-memory only (not persisted)
//

var (
	sessions     *SessionsState
	sessionsOnce sync.Once
)

func Sessions() *SessionsState {
	sessionsOnce.Do(func() {
		sessions = &SessionsState{
			Locations: make(map[string]*SessionLocation),
		}
	})
	return sessions
}

//
// SaveAll / LoadAll - convenience functions
//

func LoadAll() error {
	var errs []error
	if err := Subscriptions().Load(); err != nil {
		errs = append(errs, err)
	}
	if err := Notifications().Load(); err != nil {
		errs = append(errs, err)
	}
	if err := Couriers().Load(); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

func SaveAll() error {
	var errs []error
	if err := Subscriptions().Save(); err != nil {
		errs = append(errs, err)
	}
	if err := Notifications().Save(); err != nil {
		errs = append(errs, err)
	}
	if err := Couriers().Save(); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

func SaveAllAsync() {
	go func() {
		if err := SaveAll(); err != nil {
			log.Printf("[data] SaveAll error: %v", err)
		}
	}()
}

// StartBackgroundSave starts periodic saves
func StartBackgroundSave(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			if err := SaveAll(); err != nil {
				log.Printf("[data] Background save error: %v", err)
			}
		}
	}()
	log.Printf("[data] Background save started (every %v)", interval)
}

//
// JSON helpers
//

func loadJSON(path string, v interface{}) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func saveJSON(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Timestamp helpers
func now() int64 {
	return time.Now().Unix()
}

func hoursAgo(hours int) int64 {
	return time.Now().Add(-time.Duration(hours) * time.Hour).Unix()
}
