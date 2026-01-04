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

// Name holds a Name of Allah
type Name struct {
	Number      int      `json:"number"`
	English     string   `json:"english"`
	Arabic      string   `json:"arabic"`
	Meaning     string   `json:"meaning"`
	Description string   `json:"description"`
	Summary     string   `json:"summary"`
	Location    []string `json:"location"`
}

var (
	// Cache for names
	nameCache   = make(map[int]*Name)
	nameCacheMu sync.RWMutex
)

// GetName fetches a Name of Allah by number (1-99)
func GetName(number int) *Name {
	nameCacheMu.RLock()
	if n, ok := nameCache[number]; ok {
		nameCacheMu.RUnlock()
		return n
	}
	nameCacheMu.RUnlock()
	
	nameCacheMu.Lock()
	defer nameCacheMu.Unlock()
	
	// Double-check
	if n, ok := nameCache[number]; ok {
		return n
	}
	
	url := fmt.Sprintf("https://reminder.dev/api/names/%d", number)
	resp, err := http.Get(url)
	if err != nil {
		log.Printf("[reminder] fetch name %d error: %v", number, err)
		return nil
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		log.Printf("[reminder] name %d API returned %d", number, resp.StatusCode)
		return nil
	}
	
	var n Name
	if err := json.NewDecoder(resp.Body).Decode(&n); err != nil {
		log.Printf("[reminder] decode name %d error: %v", number, err)
		return nil
	}
	
	nameCache[number] = &n
	log.Printf("[reminder] cached name %d: %s", number, n.English)
	
	return &n
}

// TimeBasedReminder represents a curated reminder for a specific time
type TimeBasedReminder struct {
	Type     string // "surah", "name", "verse"
	Number   int    // Surah number or Name number
	Verses   []int  // For surahs, which verses to show (empty = first 3)
	Reason   string // Why this reminder at this time
}

// Curated time-based reminders
var timeReminders = map[string]TimeBasedReminder{
	// Morning reminders (Fajr to sunrise)
	"fajr": {
		Type:   "surah",
		Number: 89, // Al-Fajr (The Dawn)
		Verses: []int{1, 2, 3, 4},
		Reason: "By the dawn...",
	},
	// Duha time (after sunrise, before Dhuhr)
	"duha": {
		Type:   "surah",
		Number: 93, // Ad-Duhaa (The Morning Hours)
		Verses: []int{1, 2, 3},
		Reason: "By the morning sunlight...",
	},
	// Midday - The Provider
	"dhuhr": {
		Type:   "name",
		Number: 17, // Ar-Razzaq (The Provider)
		Reason: "Midday reminder of provision",
	},
	// Afternoon - The Designer
	"asr": {
		Type:   "name",
		Number: 13, // Al-Musawwir (The Fashioner)
		Reason: "Afternoon reflection on creation",
	},
	// Evening - The Light
	"maghrib": {
		Type:   "name",
		Number: 93, // An-Nur (The Light)
		Reason: "As day turns to night",
	},
	// Night
	"isha": {
		Type:   "surah",
		Number: 92, // Al-Layl (The Night)
		Verses: []int{1, 2, 3, 4},
		Reason: "By the night when it covers...",
	},
	// Eternal Refuge - for difficult moments
	"refuge": {
		Type:   "name",
		Number: 68, // As-Samad (The Eternal Refuge)
		Reason: "The Self-Sufficient, upon whom all depend",
	},
	// The Creator - seeing nature
	"creator": {
		Type:   "name",
		Number: 11, // Al-Khaliq (The Creator)
		Reason: "Reminder of creation",
	},
}

// GetTimeReminder returns a reminder for a specific time/context
func GetTimeReminder(key string) *Reminder {
	tr, ok := timeReminders[key]
	if !ok {
		return nil
	}
	
	switch tr.Type {
	case "surah":
		return getSurahReminder(tr.Number, tr.Verses)
	case "name":
		return getNameReminder(tr.Number)
	default:
		return nil
	}
}

func getSurahReminder(number int, verses []int) *Reminder {
	s := GetSurah(number)
	if s == nil || len(s.Verses) < 2 {
		return nil
	}
	
	// Default to verses 1-3 if not specified
	if len(verses) == 0 {
		verses = []int{1, 2, 3}
	}
	
	// Build verse text (skip bismillah at index 0)
	verseText := ""
	for _, v := range verses {
		if v < len(s.Verses) {
			if verseText != "" {
				verseText += " "
			}
			verseText += s.Verses[v].Text
		}
	}
	
	// Format reference
	ref := fmt.Sprintf("%s - %d:%d", s.Name, number, verses[0])
	if len(verses) > 1 {
		ref = fmt.Sprintf("%s - %d:%d-%d", s.Name, number, verses[0], verses[len(verses)-1])
	}
	
	return &Reminder{
		Verse: fmt.Sprintf("%s\n\n%s", ref, verseText),
	}
}

func getNameReminder(number int) *Reminder {
	n := GetName(number)
	if n == nil {
		return nil
	}
	
	return &Reminder{
		Name: fmt.Sprintf("%s - %s - %s\n\n%s", n.English, n.Arabic, n.Meaning, n.Description),
	}
}

// GetDuhaReminder returns a reminder from Surah Ad-Duha (93)
// Best shown at Duha time (after sunrise until before Dhuhr)
func GetDuhaReminder() *Reminder {
	return GetTimeReminder("duha")
}
