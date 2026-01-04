package spatial

import (
	"encoding/json"
	"fmt"
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

// Surah holds a chapter from the Quran
type Surah struct {
	Name   string  `json:"name"`
	Number int     `json:"number"`
	Verses []Verse `json:"verses"`
}

// Verse holds a single verse
type Verse struct {
	Chapter  int    `json:"chapter"`
	Number   int    `json:"number"`
	Text     string `json:"text"`
	Arabic   string `json:"arabic"`
	Comments string `json:"comments"`
}

var (
	cachedReminder *Reminder
	reminderMu     sync.RWMutex
	reminderDate   string
	
	// Cache for surahs
	surahCache   = make(map[int]*Surah)
	surahCacheMu sync.RWMutex
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

// GetSurah fetches a specific surah by number
func GetSurah(number int) *Surah {
	surahCacheMu.RLock()
	if s, ok := surahCache[number]; ok {
		surahCacheMu.RUnlock()
		return s
	}
	surahCacheMu.RUnlock()
	
	surahCacheMu.Lock()
	defer surahCacheMu.Unlock()
	
	// Double-check
	if s, ok := surahCache[number]; ok {
		return s
	}
	
	url := fmt.Sprintf("https://reminder.dev/api/quran/%d", number)
	resp, err := http.Get(url)
	if err != nil {
		log.Printf("[reminder] fetch surah %d error: %v", number, err)
		return nil
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		log.Printf("[reminder] surah %d API returned %d", number, resp.StatusCode)
		return nil
	}
	
	var s Surah
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		log.Printf("[reminder] decode surah %d error: %v", number, err)
		return nil
	}
	
	surahCache[number] = &s
	log.Printf("[reminder] cached surah %d: %s", number, s.Name)
	
	return &s
}

// GetDuhaReminder returns a reminder from Surah Ad-Duha (93)
// Best shown at Duha time (after sunrise until before Dhuhr)
func GetDuhaReminder() *Reminder {
	s := GetSurah(93)
	if s == nil || len(s.Verses) < 3 {
		return nil
	}
	
	// Get verses 1-3 (skip bismillah at index 0)
	verseText := ""
	for i := 1; i <= 3 && i < len(s.Verses); i++ {
		if verseText != "" {
			verseText += " "
		}
		verseText += s.Verses[i].Text
	}
	
	return &Reminder{
		Verse: fmt.Sprintf("%s - 93:1-3\n\n%s", s.Name, verseText),
	}
}
