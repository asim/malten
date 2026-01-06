package server

import (
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// NotificationDedupe tracks sent notifications per session
type NotificationDedupe struct {
	mu       sync.RWMutex
	sessions map[string]*SessionDedupe // session_id -> dedupe state
}

type SessionDedupe struct {
	SentContent map[string]int64 // content_key -> unix timestamp
}

var notificationDedupe = &NotificationDedupe{
	sessions: make(map[string]*SessionDedupe),
}

// GetDedupe returns the global dedupe tracker
func GetDedupe() *NotificationDedupe {
	return notificationDedupe
}

// ShouldSend checks if we should send this content to this session
// Returns true if not recently sent, and marks it as sent
func (d *NotificationDedupe) ShouldSend(sessionID, content string) bool {
	contentKey := ExtractContentKey(content)
	
	d.mu.Lock()
	defer d.mu.Unlock()
	
	sess, exists := d.sessions[sessionID]
	if !exists {
		sess = &SessionDedupe{SentContent: make(map[string]int64)}
		d.sessions[sessionID] = sess
	}
	
	// Check if recently sent (within 6 hours)
	if timestamp, exists := sess.SentContent[contentKey]; exists {
		if time.Now().Unix()-timestamp < 6*60*60 {
			return false // Already sent recently
		}
	}
	
	// Mark as sent
	sess.SentContent[contentKey] = time.Now().Unix()
	
	// Clean old entries
	cutoff := time.Now().Unix() - 24*60*60
	for k, v := range sess.SentContent {
		if v < cutoff {
			delete(sess.SentContent, k)
		}
	}
	
	return true
}

// ExtractContentKey extracts a dedupe key from message content
func ExtractContentKey(message string) string {
	lower := strings.ToLower(message)
	
	// Rain notifications: key is "rain_<hour>" or "rain_now"
	if strings.Contains(lower, "rain") || strings.Contains(lower, "🌧") {
		if strings.Contains(lower, "now") || strings.Contains(lower, "currently") || strings.Contains(lower, "likely now") {
			return "rain_now"
		}
		// Extract hour from message
		// Patterns: "at 9 PM", "at 21:00", "at 9pm", "around 14:30"
		re := regexp.MustCompile(`(?:at |around )(\d{1,2})(?::\d{2})?\s*(?:PM|AM|pm|am)?`)
		if matches := re.FindStringSubmatch(message); len(matches) > 1 {
			hour := matches[1]
			// Normalize to 24h
			if strings.Contains(lower, "pm") {
				if h, _ := strconv.Atoi(hour); h < 12 {
					hour = strconv.Itoa(h + 12)
				}
			}
			return "rain_" + hour
		}
		return "rain_general"
	}
	
	// Temperature warnings
	if strings.Contains(lower, "❄️") || strings.Contains(lower, "🌡️") || strings.Contains(lower, "temperature") {
		return "temp_warning"
	}
	
	// Prayer notifications
	if strings.Contains(lower, "🕌") || strings.Contains(lower, "prayer") || 
	   strings.Contains(lower, "fajr") || strings.Contains(lower, "dhuhr") ||
	   strings.Contains(lower, "asr") || strings.Contains(lower, "maghrib") ||
	   strings.Contains(lower, "isha") {
		// Extract prayer name
		prayers := []string{"fajr", "dhuhr", "asr", "maghrib", "isha"}
		for _, p := range prayers {
			if strings.Contains(lower, p) {
				return "prayer_" + p
			}
		}
		return "prayer_general"
	}
	
	// Default: use first 50 chars
	key := lower
	if len(key) > 50 {
		key = key[:50]
	}
	return "msg_" + key
}

// LoadFromPush loads existing pushed_content from push subscriptions
func (d *NotificationDedupe) LoadFromPush(sessions map[string]map[string]int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	
	for sessionID, content := range sessions {
		d.sessions[sessionID] = &SessionDedupe{SentContent: content}
	}
}

// GetAllState returns a copy of all session dedupe state (for persistence)
func (d *NotificationDedupe) GetAllState() map[string]map[string]int64 {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	result := make(map[string]map[string]int64)
	for sessionID, sess := range d.sessions {
		if sess.SentContent != nil && len(sess.SentContent) > 0 {
			copy := make(map[string]int64)
			for k, v := range sess.SentContent {
				copy[k] = v
			}
			result[sessionID] = copy
		}
	}
	return result
}
