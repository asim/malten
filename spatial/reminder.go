package spatial

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
)

// Reminder holds the daily reminder from reminder.dev
type Reminder struct {
	Date    string            `json:"date"`
	Hijri   string            `json:"hijri"`
	Verse   string            `json:"verse"`
	Name    string            `json:"name"`
	Hadith  string            `json:"hadith"`
	Message string            `json:"message"`
	Links   map[string]string `json:"links"`
	Updated string            `json:"updated"`
}

var (
	cachedReminder *Reminder
	reminderMu     sync.RWMutex
	reminderDate   string
)

// GetDailyReminder returns today's reminder, fetching if needed
func GetDailyReminder() *Reminder {
	today := time.Now().Format("2006-01-02")
	
	reminderMu.RLock()
	if cachedReminder != nil && reminderDate == today {
		r := cachedReminder
		reminderMu.RUnlock()
		return r
	}
	reminderMu.RUnlock()
	
	// Fetch fresh
	reminderMu.Lock()
	defer reminderMu.Unlock()
	
	// Double-check after acquiring write lock
	if cachedReminder != nil && reminderDate == today {
		return cachedReminder
	}
	
	resp, err := http.Get("https://reminder.dev/api/daily")
	if err != nil {
		log.Printf("[reminder] fetch error: %v", err)
		return cachedReminder // Return stale if available
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		log.Printf("[reminder] API returned %d", resp.StatusCode)
		return cachedReminder
	}
	
	var r Reminder
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		log.Printf("[reminder] decode error: %v", err)
		return cachedReminder
	}
	
	cachedReminder = &r
	reminderDate = today
	log.Printf("[reminder] fetched daily reminder: %s", r.Hijri)
	
	return &r
}
