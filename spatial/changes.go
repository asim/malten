package spatial

import (
	"fmt"
	"sync"
)

// SessionContext tracks last context sent to each session
var (
	sessionContexts   = make(map[string]*ContextData)
	sessionContextsMu sync.RWMutex
)

// ContextChanges represents what changed between two contexts
type ContextChanges struct {
	LocationChanged bool
	NewLocation     string
	PrayerChanged   bool
	NewPrayer       string
	WeatherChanged  bool
	NewWeather      string
	RainWarning     string // Non-empty if new rain warning
	BusArriving     string // Non-empty if bus arriving soon (<3 min)
}

// DetectChanges compares old and new context, returns what changed meaningfully
func DetectChanges(old, new *ContextData) *ContextChanges {
	changes := &ContextChanges{}
	
	if old == nil {
		// First context - show location
		if new.Location != nil {
			changes.LocationChanged = true
			changes.NewLocation = new.Location.Name
		}
		return changes
	}
	
	// Location changed (street level)
	if new.Location != nil && old.Location != nil {
		oldStreet := extractStreet(old.Location.Name)
		newStreet := extractStreet(new.Location.Name)
		if newStreet != oldStreet && newStreet != "" {
			changes.LocationChanged = true
			changes.NewLocation = newStreet
		}
	}
	
	// Prayer changed
	if new.Prayer != nil && old.Prayer != nil {
		if new.Prayer.Current != old.Prayer.Current {
			changes.PrayerChanged = true
			changes.NewPrayer = new.Prayer.Display
		}
	}
	
	// Weather: only care about significant temp change (>3°) or condition change
	if new.Weather != nil && old.Weather != nil {
		tempDiff := new.Weather.Temp - old.Weather.Temp
		if tempDiff > 3 || tempDiff < -3 {
			changes.WeatherChanged = true
			changes.NewWeather = new.Weather.Condition
		}
	}
	
	// Rain warning: only if new
	if new.Weather != nil && new.Weather.RainWarning != "" {
		if old.Weather == nil || old.Weather.RainWarning != new.Weather.RainWarning {
			changes.RainWarning = new.Weather.RainWarning
		}
	}
	
	// Bus arriving soon (would need bus info in context)
	// TODO: implement when bus data is in ContextData
	
	return changes
}

// GetSessionContext gets the last context sent to a session
func GetSessionContext(session string) *ContextData {
	sessionContextsMu.RLock()
	defer sessionContextsMu.RUnlock()
	return sessionContexts[session]
}

// SetSessionContext stores context for a session
func SetSessionContext(session string, ctx *ContextData) {
	sessionContextsMu.Lock()
	defer sessionContextsMu.Unlock()
	sessionContexts[session] = ctx
}

// GetContextWithChanges returns context and any meaningful changes to push
func GetContextWithChanges(session string, lat, lon float64) (*ContextData, []string) {
	old := GetSessionContext(session)
	new := GetContextData(lat, lon)
	
	changes := DetectChanges(old, new)
	SetSessionContext(session, new)
	
	var messages []string
	
	if changes.LocationChanged && changes.NewLocation != "" {
		messages = append(messages, fmt.Sprintf("📍 %s", changes.NewLocation))
	}
	
	if changes.PrayerChanged && changes.NewPrayer != "" {
		messages = append(messages, fmt.Sprintf("🕌 %s", changes.NewPrayer))
	}
	
	if changes.RainWarning != "" {
		messages = append(messages, changes.RainWarning)
	}
	
	if changes.WeatherChanged && changes.NewWeather != "" {
		messages = append(messages, fmt.Sprintf("🌡️ %s", changes.NewWeather))
	}
	
	return new, messages
}

func extractStreet(location string) string {
	// "Milton Road, TW12 2LL" -> "Milton Road"
	if location == "" {
		return ""
	}
	parts := splitOnComma(location)
	if len(parts) > 0 {
		return parts[0]
	}
	return location
}

func splitOnComma(s string) []string {
	var parts []string
	current := ""
	for _, c := range s {
		if c == ',' {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	// Trim whitespace
	for i := range parts {
		parts[i] = trimSpace(parts[i])
	}
	return parts
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
