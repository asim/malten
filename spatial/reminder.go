package spatial

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

// Reminder holds the daily reminder from reminder.dev
type Reminder struct {
	Date       string            `json:"date"`
	Hijri      string            `json:"hijri"`
	Verse      string            `json:"verse"`
	Name       string            `json:"name"`
	NameNumber int               `json:"name_number,omitempty"` // For linking to reminder.dev/name/{number}
	Hadith     string            `json:"hadith"`
	Message    string            `json:"message"`
	Links      map[string]string `json:"links"`
	Updated    string            `json:"updated"`
	// Additional fields for time-based reminders
	Title string `json:"title,omitempty"`
	Emoji string `json:"emoji,omitempty"`
	Image string `json:"image,omitempty"` // URL to nature image
}

// GetNameNumber extracts the name number from the Links map or returns the NameNumber field
func (r *Reminder) GetNameNumber() int {
	if r.NameNumber > 0 {
		return r.NameNumber
	}
	// Try to extract from links.name (e.g., "/names/31" -> 31)
	if nameLink, ok := r.Links["name"]; ok {
		var num int
		if _, err := fmt.Sscanf(nameLink, "/names/%d", &num); err == nil {
			return num
		}
		// Also try /name/ format
		if _, err := fmt.Sscanf(nameLink, "/name/%d", &num); err == nil {
			return num
		}
	}
	return 0
}

// Surah holds a chapter from the Quran
type Surah struct {
	Name    string  `json:"name"`
	Number  int     `json:"number"`
	English string  `json:"english"` // English name (e.g. "The Declining Day")
	Verses  []Verse `json:"verses"`
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

	// Check cache first
	reminderMu.RLock()
	if cachedReminder != nil && reminderDate == today {
		r := cachedReminder
		reminderMu.RUnlock()
		return r
	}
	reminderMu.RUnlock()

	// Fetch WITHOUT holding lock (network call)
	resp, err := External.Get("reminder", "https://reminder.dev/api/daily")
	if err != nil {
		log.Printf("[reminder] fetch error: %v", err)
		reminderMu.RLock()
		r := cachedReminder
		reminderMu.RUnlock()
		return r // Return stale if available
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("[reminder] API returned %d", resp.StatusCode)
		reminderMu.RLock()
		r := cachedReminder
		reminderMu.RUnlock()
		return r
	}

	var r Reminder
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		log.Printf("[reminder] decode error: %v", err)
		reminderMu.RLock()
		stale := cachedReminder
		reminderMu.RUnlock()
		return stale
	}

	// Store in cache
	reminderMu.Lock()
	cachedReminder = &r
	reminderDate = today
	reminderMu.Unlock()

	log.Printf("[reminder] fetched daily reminder: %s", r.Hijri)
	return &r
}

// GetSurah fetches a specific surah by number
func GetSurah(number int) *Surah {
	// Check cache first
	surahCacheMu.RLock()
	if s, ok := surahCache[number]; ok {
		surahCacheMu.RUnlock()
		return s
	}
	surahCacheMu.RUnlock()

	// Fetch WITHOUT holding lock (network call)
	url := fmt.Sprintf("https://reminder.dev/api/quran/%d", number)
	resp, err := External.Get("reminder", url)
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

	// Store in cache
	surahCacheMu.Lock()
	surahCache[number] = &s
	surahCacheMu.Unlock()

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
	// Check cache first
	nameCacheMu.RLock()
	if n, ok := nameCache[number]; ok {
		nameCacheMu.RUnlock()
		return n
	}
	nameCacheMu.RUnlock()

	// Fetch WITHOUT holding lock (network call)
	url := fmt.Sprintf("https://reminder.dev/api/names/%d", number)
	resp, err := External.Get("reminder", url)
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

	// Store in cache
	nameCacheMu.Lock()
	nameCache[number] = &n
	nameCacheMu.Unlock()
	log.Printf("[reminder] cached name %d: %s", number, n.English)

	return &n
}

// TimeBasedReminder represents a curated reminder for a specific time
type TimeBasedReminder struct {
	Type      string // "surah", "name", "verse"
	Number    int    // Surah number or Name number
	Verses    []int  // For surahs, which verses to show (empty = first 3)
	Reason    string // Why this reminder at this time
	ImageType string // For FetchNatureImage: "sunrise", "morning", "mountains", "evening", "sunset", "moon", "stars"
	Title     string // Display title (e.g. "Dawn", "The Morning Light")
	Emoji     string // Emoji for display
}

// Curated time-based reminders
var timeReminders = map[string]TimeBasedReminder{
	"fajr": {
		Type:      "surah",
		Number:    89, // Al-Fajr (The Dawn)
		Verses:    []int{1, 2, 3, 4},
		Title:     "Dawn",
		Emoji:     "🌅",
		ImageType: "sunrise",
	},
	"duha": {
		Type:      "surah",
		Number:    93, // Ad-Duhaa (The Morning Hours)
		Verses:    []int{1, 2, 3},
		Title:     "The Morning Light",
		Emoji:     "☀️",
		ImageType: "morning",
	},
	"dhuhr": {
		Type:      "name",
		Number:    17, // Ar-Razzaq (The Provider)
		Title:     "Midday",
		Emoji:     "🏔️",
		ImageType: "mountains",
	},
	"asr": {
		Type:      "surah",
		Number:    103, // Al-Asr (The Declining Day)
		Verses:    []int{1, 2, 3},
		Title:     "The Declining Day",
		Emoji:     "📖",
		ImageType: "evening",
	},
	"maghrib": {
		Type:      "name",
		Number:    93, // An-Nur (The Light)
		Title:     "Sunset",
		Emoji:     "🌇",
		ImageType: "sunset",
	},
	"isha": {
		Type:      "surah",
		Number:    92, // Al-Layl (The Night)
		Verses:    []int{1, 2, 3, 4},
		Title:     "Night",
		Emoji:     "🌙",
		ImageType: "moon",
	},
	"night": {
		Type:      "surah",
		Number:    86, // At-Tariq (The Nightcommer)
		Verses:    []int{1, 2, 3},
		Title:     "The Stars",
		Emoji:     "✨",
		ImageType: "stars",
	},
	"refuge": {
		Type:      "name",
		Number:    68, // As-Samad (The Eternal Refuge)
		Title:     "Eternal Refuge",
		Emoji:     "🖤",
		ImageType: "",
	},
	"creator": {
		Type:      "name",
		Number:    11, // Al-Khaliq (The Creator)
		Title:     "The Creator",
		Emoji:     "🌿",
		ImageType: "nature",
	},
}

// GetTimeReminder returns a reminder for a specific time/context
func GetTimeReminder(key string) *Reminder {
	tr, ok := timeReminders[key]
	if !ok {
		return nil
	}

	var r *Reminder
	switch tr.Type {
	case "surah":
		r = getSurahReminder(tr.Number, tr.Verses)
	case "name":
		r = getNameReminder(tr.Number)
	default:
		return nil
	}

	if r == nil {
		return nil
	}

	// Add title, emoji, and image from config
	r.Title = tr.Title
	r.Emoji = tr.Emoji
	if tr.ImageType != "" {
		r.Image = FetchNatureImage(tr.ImageType)
	}

	return r
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
		Name:       fmt.Sprintf("%s - %s - %s\n\n%s", n.English, n.Arabic, n.Meaning, n.Description),
		NameNumber: number,
	}
}

// GetDuhaReminder returns a reminder from Surah Ad-Duha (93)
// Best shown at Duha time (after sunrise until before Dhuhr)
func GetDuhaReminder() *Reminder {
	return GetTimeReminder("duha")
}
