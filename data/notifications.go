package data

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// NotificationsFile manages notification_state.json
type NotificationsFile struct {
	mu          sync.RWMutex
	History     map[string][]HistoryItem    `json:"history"`
	ContentKeys map[string]map[string]int64 `json:"content_keys"`
}

// HistoryItem is a sent notification with timestamp
type HistoryItem struct {
	Text      string `json:"text"`
	Timestamp int64  `json:"ts"`
}

// Load reads from notification_state.json
func (n *NotificationsFile) Load() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	return loadJSON(filepath.Join(dataDir, "notification_state.json"), n)
}

// Save writes to notification_state.json
func (n *NotificationsFile) Save() error {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return saveJSON(filepath.Join(dataDir, "notification_state.json"), n)
}

// AddToHistory records a sent notification
func (n *NotificationsFile) AddToHistory(sessionID, text string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.History[sessionID] == nil {
		n.History[sessionID] = []HistoryItem{}
	}

	n.History[sessionID] = append(n.History[sessionID], HistoryItem{
		Text:      text,
		Timestamp: now(),
	})

	// Keep only last 20
	if len(n.History[sessionID]) > 20 {
		n.History[sessionID] = n.History[sessionID][len(n.History[sessionID])-20:]
	}
}

// GetRecentHistory returns notifications from last N hours
func (n *NotificationsFile) GetRecentHistory(sessionID string, hours int) []string {
	n.mu.RLock()
	defer n.mu.RUnlock()

	items := n.History[sessionID]
	if len(items) == 0 {
		return nil
	}

	cutoff := hoursAgo(hours)
	var recent []string
	for _, item := range items {
		if item.Timestamp > cutoff {
			recent = append(recent, item.Text)
		}
	}
	return recent
}

// ShouldSend checks rule-based dedupe and marks as sent if allowed
func (n *NotificationsFile) ShouldSend(sessionID, content string) bool {
	contentKey := ExtractContentKey(content)

	n.mu.Lock()
	defer n.mu.Unlock()

	if n.ContentKeys[sessionID] == nil {
		n.ContentKeys[sessionID] = make(map[string]int64)
	}

	// Check if recently sent (within 6 hours)
	if timestamp, exists := n.ContentKeys[sessionID][contentKey]; exists {
		if now()-timestamp < 6*60*60 {
			return false
		}
	}

	// Mark as sent
	n.ContentKeys[sessionID][contentKey] = now()

	// Clean old entries (24 hours)
	cutoff := hoursAgo(24)
	for k, v := range n.ContentKeys[sessionID] {
		if v < cutoff {
			delete(n.ContentKeys[sessionID], k)
		}
	}

	return true
}

// ExtractContentKey creates a dedupe key from message content
func ExtractContentKey(message string) string {
	lower := strings.ToLower(message)

	if strings.Contains(lower, "rain") || strings.Contains(lower, "🌧") {
		if strings.Contains(lower, "now") || strings.Contains(lower, "currently") || strings.Contains(lower, "likely now") {
			return "rain_now"
		}
		re := regexp.MustCompile(`(?:at |around )(\d{1,2})(?::\d{2})?\s*(?:PM|AM|pm|am)?`)
		if matches := re.FindStringSubmatch(message); len(matches) > 1 {
			hour := matches[1]
			if strings.Contains(lower, "pm") {
				if h, _ := strconv.Atoi(hour); h < 12 {
					hour = strconv.Itoa(h + 12)
				}
			}
			return "rain_" + hour
		}
		return "rain_general"
	}

	if strings.Contains(lower, "❄️") || strings.Contains(lower, "🌡️") || strings.Contains(lower, "temperature") {
		return "temp_warning"
	}

	key := "msg_" + lower
	if len(key) > 50 {
		key = key[:50]
	}
	return key
}

// LoadFromPush loads PushedContent from old push format (migration)
func (n *NotificationsFile) LoadFromPush(sessions map[string]map[string]int64) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for sessionID, content := range sessions {
		n.ContentKeys[sessionID] = content
	}
}

// GetAllState returns content keys (for compatibility)
func (n *NotificationsFile) GetAllState() map[string]map[string]int64 {
	n.mu.RLock()
	defer n.mu.RUnlock()
	result := make(map[string]map[string]int64)
	for sessionID, keys := range n.ContentKeys {
		if keys != nil && len(keys) > 0 {
			copy := make(map[string]int64)
			for k, v := range keys {
				copy[k] = v
			}
			result[sessionID] = copy
		}
	}
	return result
}

// LoadHistoryFromJSON loads from old format (migration)
func (n *NotificationsFile) LoadHistoryFromJSON(jsonData []byte) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	var history map[string][]HistoryItem
	if err := json.Unmarshal(jsonData, &history); err != nil {
		return err
	}
	for sessionID, items := range history {
		n.History[sessionID] = items
	}
	return nil
}

// SaveHistoryToJSON saves history (compatibility)
func (n *NotificationsFile) SaveHistoryToJSON() ([]byte, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return json.Marshal(n.History)
}
