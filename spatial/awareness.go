package spatial

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

// Observation represents something an agent noticed
type Observation struct {
	Time     time.Time              `json:"time"`
	Type     string                 `json:"type"`
	AgentID  string                 `json:"agent_id"`
	AgentName string                `json:"agent_name"`
	Data     map[string]interface{} `json:"data"`
	Surfaced bool                   `json:"surfaced"`
}

// Observation types
const (
	ObsWeatherChange    = "weather_change"
	ObsWeatherWarning   = "weather_warning"
	ObsNewPlace         = "new_place"
	ObsDisruption       = "disruption"
	ObsArrivalAnomaly   = "arrival_anomaly"
	ObsPrayerApproaching = "prayer_approaching"
	ObsNotableNearby    = "notable_nearby"
)

// ObservationLog accumulates observations per agent
type ObservationLog struct {
	mu           sync.RWMutex
	observations map[string][]*Observation // agentID -> observations
	lastProcess  map[string]time.Time      // agentID -> last awareness processing
}

var observationLog = &ObservationLog{
	observations: make(map[string][]*Observation),
	lastProcess:  make(map[string]time.Time),
}

// GetObservationLog returns the global observation log
func GetObservationLog() *ObservationLog {
	return observationLog
}

// Add adds an observation for an agent
func (o *ObservationLog) Add(agentID, agentName, obsType string, data map[string]interface{}) {
	o.mu.Lock()
	defer o.mu.Unlock()

	obs := &Observation{
		Time:      time.Now(),
		Type:      obsType,
		AgentID:   agentID,
		AgentName: agentName,
		Data:      data,
	}

	o.observations[agentID] = append(o.observations[agentID], obs)

	// Keep only last 50 observations per agent
	if len(o.observations[agentID]) > 50 {
		o.observations[agentID] = o.observations[agentID][len(o.observations[agentID])-50:]
	}

	log.Printf("[awareness] %s: %s - %v", agentName, obsType, truncateMap(data))
}

// GetPending returns unsurfaced observations for an agent
func (o *ObservationLog) GetPending(agentID string) []*Observation {
	o.mu.RLock()
	defer o.mu.RUnlock()

	var pending []*Observation
	for _, obs := range o.observations[agentID] {
		if !obs.Surfaced {
			pending = append(pending, obs)
		}
	}
	return pending
}

// MarkSurfaced marks observations as surfaced
func (o *ObservationLog) MarkSurfaced(agentID string, types []string) {
	o.mu.Lock()
	defer o.mu.Unlock()

	typeSet := make(map[string]bool)
	for _, t := range types {
		typeSet[t] = true
	}

	for _, obs := range o.observations[agentID] {
		if typeSet[obs.Type] {
			obs.Surfaced = true
		}
	}
}

// ShouldProcess checks if enough time has passed since last processing
func (o *ObservationLog) ShouldProcess(agentID string, hasActiveUsers bool) bool {
	o.mu.RLock()
	defer o.mu.RUnlock()

	last, ok := o.lastProcess[agentID]
	if !ok {
		return true
	}

	// More frequent processing if users are present
	interval := 10 * time.Minute
	if hasActiveUsers {
		interval = 5 * time.Minute
	}

	return time.Since(last) >= interval
}

// SetProcessed marks agent as processed
func (o *ObservationLog) SetProcessed(agentID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.lastProcess[agentID] = time.Now()
}

// AwarenessItem is something worth telling the user
type AwarenessItem struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Emoji   string `json:"emoji"`
}

// ProcessAwareness runs the LLM filter on pending observations
func ProcessAwareness(agentID, agentName string, userContext map[string]interface{}) ([]AwarenessItem, error) {
	obsLog := GetObservationLog()
	
	pending := obsLog.GetPending(agentID)
	if len(pending) == 0 {
		return nil, nil
	}

	// Build observations summary
	var obsSummary []string
	for _, obs := range pending {
		obsSummary = append(obsSummary, fmt.Sprintf("- %s: %s %v", 
			obs.Time.Format("15:04"), obs.Type, obs.Data))
	}

	// Build prompt
	prompt := fmt.Sprintf(`You are an awareness filter for a spatial app. Your job is to decide what's worth telling the user.

LOCATION: %s
TIME: %s

RECENT OBSERVATIONS:
%s

USER CONTEXT:
%v

RULES:
- Only surface genuinely interesting/useful things
- Normal weather, normal bus times = NOT interesting
- Rain starting, disruption on usual route, new place = potentially interesting
- Be selective - users don't want spam
- If nothing is noteworthy, return empty array

Respond with JSON array of items to surface:
[{"type": "...", "message": "...", "emoji": "..."}]

Or empty array if nothing worth surfacing:
[]

Examples:
- Rain starting in 20 min → [{"type": "weather", "message": "Rain starting around 14:30", "emoji": "🌧️"}]
- Normal sunny weather → []
- Bus 3 min late → []
- Major disruption on A316 → [{"type": "disruption", "message": "Delays on A316 - consider alternate route", "emoji": "⚠️"}]
- New cafe on your usual street → [{"type": "new_place", "message": "New cafe 'Grind & Steam' opened on Milton Road", "emoji": "☕"}]

Respond ONLY with JSON array, nothing else.`,
		agentName,
		time.Now().Format("Monday 15:04"),
		strings.Join(obsSummary, "\n"),
		userContext,
	)

	// Call LLM
	client := getAgentLLMClient()
	if client == nil {
		return nil, fmt.Errorf("no LLM client")
	}

	var resp openai.ChatCompletionResponse
	err := LLMRateLimitedCall(func() error {
		var llmErr error
		resp, llmErr = client.CreateChatCompletion(
			context.Background(),
			openai.ChatCompletionRequest{
				Model: getAgentModel(),
				Messages: []openai.ChatCompletionMessage{
					{Role: openai.ChatMessageRoleUser, Content: prompt},
				},
				MaxTokens: 256,
			},
		)
		return llmErr
	})
	if err != nil {
		return nil, fmt.Errorf("LLM error: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no LLM response")
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	obsLog.SetProcessed(agentID)

	// Parse response
	var items []AwarenessItem
	
	// Find JSON array in response
	start := strings.Index(content, "[")
	end := strings.LastIndex(content, "]")
	if start >= 0 && end > start {
		jsonStr := content[start : end+1]
		if err := json.Unmarshal([]byte(jsonStr), &items); err != nil {
			log.Printf("[awareness] Failed to parse LLM response: %v", err)
			return nil, nil
		}
	}

	// Mark observations as surfaced if we got items
	if len(items) > 0 {
		var types []string
		for _, obs := range pending {
			types = append(types, obs.Type)
		}
		observationLog.MarkSurfaced(agentID, types)
	}

	return items, nil
}

// Helper to truncate map for logging
func truncateMap(m map[string]interface{}) string {
	b, _ := json.Marshal(m)
	s := string(b)
	if len(s) > 60 {
		return s[:57] + "..."
	}
	return s
}

// AddWeatherObservation adds a weather observation if conditions are notable
func AddWeatherObservation(agentID, agentName string, temp float64, condition string, rainWarning string) {
	// Only observe if there's something notable
	if rainWarning != "" {
		GetObservationLog().Add(agentID, agentName, ObsWeatherWarning, map[string]interface{}{
			"temp":         temp,
			"condition":    condition,
			"rain_warning": rainWarning,
		})
	}
}

// AddDisruptionObservation adds a transport disruption observation
func AddDisruptionObservation(agentID, agentName string, severity, location, description string) {
	if severity == "Severe" || severity == "Serious" {
		GetObservationLog().Add(agentID, agentName, ObsDisruption, map[string]interface{}{
			"severity":    severity,
			"location":    location,
			"description": description,
		})
	}
}

// AddNewPlaceObservation adds a new place observation
func AddNewPlaceObservation(agentID, agentName string, placeName, placeType, address string) {
	GetObservationLog().Add(agentID, agentName, ObsNewPlace, map[string]interface{}{
		"name":    placeName,
		"type":    placeType,
		"address": address,
	})
}
