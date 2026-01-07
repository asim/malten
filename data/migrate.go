package data

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
)

// Migrate loads all data files and migrates old formats
func Migrate(dir string) error {
	SetDataDir(dir)
	
	// Load from new format files
	if err := LoadAll(); err != nil {
		log.Printf("[data] Warning loading data: %v", err)
	}
	
	// Migrate old notification_history.json if exists
	migrateNotificationHistory(dir)
	
	// Migrate PushedContent from old push format
	migratePushDedupe(dir)
	
	return nil
}

func migrateNotificationHistory(dir string) {
	oldFile := filepath.Join(dir, "notification_history.json")
	data, err := os.ReadFile(oldFile)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		log.Printf("[data] Warning reading old history: %v", err)
		return
	}
	
	var oldHistory map[string][]HistoryItem
	if err := json.Unmarshal(data, &oldHistory); err != nil {
		log.Printf("[data] Warning parsing old history: %v", err)
		return
	}
	
	n := Notifications()
	n.mu.Lock()
	for sessionID, items := range oldHistory {
		n.History[sessionID] = items
	}
	n.mu.Unlock()
	
	log.Printf("[data] Migrated notification history for %d sessions", len(oldHistory))
}

func migratePushDedupe(dir string) {
	oldFile := filepath.Join(dir, "push_subscriptions.json")
	fileData, err := os.ReadFile(oldFile)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		return
	}
	
	var oldUsers []struct {
		SessionID     string           `json:"session_id"`
		PushedContent map[string]int64 `json:"pushed_content,omitempty"`
	}
	if err := json.Unmarshal(fileData, &oldUsers); err != nil {
		return
	}
	
	n := Notifications()
	n.mu.Lock()
	count := 0
	for _, u := range oldUsers {
		if u.PushedContent != nil && len(u.PushedContent) > 0 {
			n.ContentKeys[u.SessionID] = u.PushedContent
			count++
		}
	}
	n.mu.Unlock()
	
	if count > 0 {
		log.Printf("[data] Migrated dedupe keys for %d sessions", count)
	}
}
