package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/sashabaranov/go-openai"
)

// LLM client for dedupe decisions (set by main.go)
var dedupeClient *openai.Client

// SetDedupeClient sets the OpenAI client for dedupe decisions
func SetDedupeClient(client *openai.Client) {
	dedupeClient = client
}

// NotificationHistory tracks actual notification text per session
type NotificationHistory struct {
	mu       sync.RWMutex
	sessions map[string][]HistoryItem
}

type HistoryItem struct {
	Text      string `json:"text"`
	Timestamp int64  `json:"ts"`
}

var notificationHistory = &NotificationHistory{
	sessions: make(map[string][]HistoryItem),
}

// AddToHistory adds a notification to history
func AddToHistory(sessionID, text string) {
	notificationHistory.mu.Lock()
	defer notificationHistory.mu.Unlock()
	
	if notificationHistory.sessions[sessionID] == nil {
		notificationHistory.sessions[sessionID] = []HistoryItem{}
	}
	
	notificationHistory.sessions[sessionID] = append(
		notificationHistory.sessions[sessionID],
		HistoryItem{Text: text, Timestamp: time.Now().Unix()},
	)
	
	// Keep only last 20
	if len(notificationHistory.sessions[sessionID]) > 20 {
		notificationHistory.sessions[sessionID] = notificationHistory.sessions[sessionID][len(notificationHistory.sessions[sessionID])-20:]
	}
}

// GetRecentHistory returns notifications from last N hours
func GetRecentHistory(sessionID string, hours int) []string {
	notificationHistory.mu.RLock()
	defer notificationHistory.mu.RUnlock()
	
	items := notificationHistory.sessions[sessionID]
	if len(items) == 0 {
		return nil
	}
	
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour).Unix()
	var recent []string
	
	for _, item := range items {
		if item.Timestamp > cutoff {
			recent = append(recent, item.Text)
		}
	}
	
	return recent
}

// ShouldSendLLM asks an LLM whether to send a notification given history
func ShouldSendLLM(sessionID, newNotification string) bool {
	// Get recent notification history (last 6 hours)
	recentHistory := GetRecentHistory(sessionID, 6)
	
	// No history = definitely send
	if len(recentHistory) == 0 {
		AddToHistory(sessionID, newNotification)
		return true
	}
	
	// No LLM = fall back to rule-based
	if dedupeClient == nil {
		result := GetDedupe().ShouldSend(sessionID, newNotification)
		if result {
			AddToHistory(sessionID, newNotification)
		}
		return result
	}

	// Build prompt
	historyStr := ""
	for i, h := range recentHistory {
		historyStr += fmt.Sprintf("%d. %s\n", i+1, h)
	}
	
	prompt := fmt.Sprintf(`You decide whether to send a push notification.

ALREADY SENT (last 6 hours):
%s
NEW NOTIFICATION:
"%s"

Should we send this? Rules:
- Small changes (60%% vs 65%% rain) = NO
- Same info rephrased = NO  
- Genuinely new info (time changed, now happening) = YES
- Different topic = YES

Reply ONLY "yes" or "no".`, historyStr, newNotification)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := dedupeClient.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: "Fanar", // Use Fanar's default model
		Messages: []openai.ChatCompletionMessage{
			{Role: "user", Content: prompt},
		},
		MaxTokens:   5,
		Temperature: 0,
	})

	if err != nil {
		log.Printf("[dedupe-llm] Error: %v, falling back to rule-based", err)
		result := GetDedupe().ShouldSend(sessionID, newNotification)
		if result {
			AddToHistory(sessionID, newNotification)
		}
		return result
	}

	answer := strings.ToLower(strings.TrimSpace(resp.Choices[0].Message.Content))
	shouldSend := strings.HasPrefix(answer, "yes")
	
	log.Printf("[dedupe-llm] '%s' -> %s (history: %d)", 
		truncate(newNotification, 40), answer, len(recentHistory))
	
	if shouldSend {
		AddToHistory(sessionID, newNotification)
		// Also mark in rule-based for persistence
		GetDedupe().ShouldSend(sessionID, newNotification)
	}
	
	return shouldSend
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// LoadHistoryFromJSON loads history from JSON (for persistence)
func LoadHistoryFromJSON(data []byte) error {
	notificationHistory.mu.Lock()
	defer notificationHistory.mu.Unlock()
	return json.Unmarshal(data, &notificationHistory.sessions)
}

// SaveHistoryToJSON saves history to JSON
func SaveHistoryToJSON() ([]byte, error) {
	notificationHistory.mu.RLock()
	defer notificationHistory.mu.RUnlock()
	return json.Marshal(notificationHistory.sessions)
}
