package server

import (
	"log"

	"github.com/sashabaranov/go-openai"
	"malten.ai/data"
)

// LLM client (unused now, kept for compatibility)
var dedupeClient *openai.Client

// SetDedupeClient sets the LLM client (unused)
func SetDedupeClient(client *openai.Client) {
	dedupeClient = client
}

// AddToHistory delegates to data package
func AddToHistory(sessionID, text string) {
	data.Notifications().AddToHistory(sessionID, text)
}

// GetRecentHistory delegates to data package
func GetRecentHistory(sessionID string, hours int) []string {
	return data.Notifications().GetRecentHistory(sessionID, hours)
}

// ShouldSendLLM checks if notification should be sent (rule-based)
func ShouldSendLLM(sessionID, newNotification string) bool {
	result := data.Notifications().ShouldSend(sessionID, newNotification)
	if result {
		data.Notifications().AddToHistory(sessionID, newNotification)
	} else {
		log.Printf("[dedupe] Skipping duplicate for %s: %s", sessionID[:8], truncate(newNotification, 40))
	}
	return result
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// LoadHistoryFromJSON delegates to data package
func LoadHistoryFromJSON(jsonData []byte) error {
	return data.Notifications().LoadHistoryFromJSON(jsonData)
}

// SaveHistoryToJSON delegates to data package
func SaveHistoryToJSON() ([]byte, error) {
	return data.Notifications().SaveHistoryToJSON()
}
