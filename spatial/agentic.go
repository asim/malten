package spatial

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
)

// AgentState holds operational state for an agentic agent
type AgentState struct {
	LastWeatherFetch   time.Time
	LastPrayerFetch    time.Time
	LastTransportFetch time.Time
	LastPOIIndex       time.Time
	NextCycle          time.Time
	NextCycleReason    string
	ActiveUsers        int
	Events             []string // Recent events for context
}

// agentStates tracks state per agent
var agentStates = make(map[string]*AgentState)

// GetAgentState returns or creates state for an agent
func GetAgentState(agentID string) *AgentState {
	if s, ok := agentStates[agentID]; ok {
		return s
	}
	s := &AgentState{
		Events: make([]string, 0),
	}
	agentStates[agentID] = s
	return s
}

// ToolCall represents a tool the agent wants to execute
type ToolCall struct {
	Tool string                 `json:"tool"`
	Args map[string]interface{} `json:"args"`
}

// AgentCycle runs one OODA cycle for an agent:
// Observe -> Orient (LLM processes) -> Decide (LLM outputs tools) -> Act (execute tools)
func AgentCycle(agent *Entity) error {
	state := GetAgentState(agent.ID)
	db := Get()

	// Build prompt with current state and observations
	prompt := buildAgentPrompt(agent, state, db)

	log.Printf("[agent] %s cycle start, %d events queued", agent.Name, len(state.Events))

	// Get LLM client
	client := getAgentLLMClient()
	if client == nil {
		return errors.New("no LLM client configured")
	}

	// Tool execution loop - ask LLM what to do, execute, repeat until done
	maxIterations := 5
	for i := 0; i < maxIterations; i++ {
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
			return fmt.Errorf("LLM error: %w", err)
		}

		if len(resp.Choices) == 0 {
			return errors.New("no response from LLM")
		}

		content := resp.Choices[0].Message.Content
		log.Printf("[agent] %s LLM: %s", agent.Name, truncate(content, 100))

		// Parse tool call from response
		toolCall := parseToolCall(content)
		if toolCall == nil || toolCall.Tool == "" || toolCall.Tool == "done" {
			// No more actions or explicit done
			if toolCall != nil && toolCall.Tool == "done" {
				summary, _ := toolCall.Args["summary"].(string)
				log.Printf("[agent] %s cycle complete: %s", agent.Name, summary)
			}
			break
		}

		// Execute the tool
		result := executeAgentTool(agent, state, toolCall.Tool, toolCall.Args)
		log.Printf("[agent] %s tool %s: %s", agent.Name, toolCall.Tool, truncate(result, 80))

		// Add result to prompt for next iteration
		prompt += fmt.Sprintf("\n\nTool %s result: %s\n\nWhat's next? Respond ONLY with JSON.", toolCall.Tool, result)
	}

	return nil
}

func buildAgentPrompt(agent *Entity, state *AgentState, db *DB) string {
	region := GetRegion(agent.Lat, agent.Lon)
	regionName := "unknown"
	if region != nil {
		regionName = region.Name
	}

	places := db.Query(agent.Lat, agent.Lon, 2000, EntityPlace, 100)
	arrivals := db.Query(agent.Lat, agent.Lon, 500, EntityArrival, 50)

	// Build observations
	var obs []string
	now := time.Now()
	obs = append(obs, fmt.Sprintf("Time: %s", now.Format("15:04")))

	if state.LastWeatherFetch.IsZero() || now.Sub(state.LastWeatherFetch) > 10*time.Minute {
		obs = append(obs, "Weather: STALE")
	} else {
		obs = append(obs, fmt.Sprintf("Weather: fresh (%s)", formatTimeAgo(state.LastWeatherFetch)))
	}

	if state.LastPrayerFetch.IsZero() || now.Sub(state.LastPrayerFetch) > time.Hour {
		obs = append(obs, "Prayer: STALE")
	} else {
		obs = append(obs, fmt.Sprintf("Prayer: fresh (%s)", formatTimeAgo(state.LastPrayerFetch)))
	}

	if state.LastTransportFetch.IsZero() || now.Sub(state.LastTransportFetch) > 5*time.Minute {
		obs = append(obs, "Transport: STALE")
	} else {
		obs = append(obs, fmt.Sprintf("Transport: fresh (%s)", formatTimeAgo(state.LastTransportFetch)))
	}

	if state.ActiveUsers > 0 {
		obs = append(obs, fmt.Sprintf("Users: %d active", state.ActiveUsers))
	} else {
		obs = append(obs, "Users: none")
	}

	// Add queued events
	obs = append(obs, state.Events...)
	state.Events = state.Events[:0]

	return fmt.Sprintf(`You are a spatial indexing agent for %s in %s.
Keep the spatial index fresh. Places: %d, Arrivals: %d.

OBSERVATIONS:
- %s

TOOLS (respond with JSON only):
- {"tool": "fetch_weather", "args": {}}
- {"tool": "fetch_prayer", "args": {}}
- {"tool": "fetch_transport", "args": {"type": "bus|tube|rail|all"}}
- {"tool": "set_next_cycle", "args": {"minutes": N, "reason": "..."}}
- {"tool": "done", "args": {"summary": "..."}}

RULES:
- Only fetch STALE data
- If users active, fetch transport every 2-5 min
- If no users, can wait longer (set_next_cycle to 30-60 min)
- Call done when finished

What tool to run? Respond ONLY with JSON, nothing else.`,
		agent.Name, regionName, len(places), len(arrivals),
		strings.Join(obs, "\n- "))
}

func parseToolCall(content string) *ToolCall {
	content = strings.TrimSpace(content)
	
	// Find JSON object
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end < 0 || end <= start {
		return nil
	}
	
	jsonStr := content[start : end+1]
	
	var tc ToolCall
	if err := json.Unmarshal([]byte(jsonStr), &tc); err != nil {
		log.Printf("[agent] Failed to parse tool call: %v from %s", err, truncate(jsonStr, 50))
		return nil
	}
	
	return &tc
}

func executeAgentTool(agent *Entity, state *AgentState, tool string, args map[string]interface{}) string {
	switch tool {
	case "fetch_weather":
		weather := fetchWeather(agent.Lat, agent.Lon)
		if weather != nil {
			Get().Insert(weather)
			state.LastWeatherFetch = time.Now()
			return fmt.Sprintf("Fetched: %s", weather.Name)
		}
		return "Cache hit"

	case "fetch_prayer":
		prayer := fetchPrayerTimes(agent.Lat, agent.Lon)
		if prayer != nil {
			Get().Insert(prayer)
			state.LastPrayerFetch = time.Now()
			return fmt.Sprintf("Fetched: %s", prayer.Name)
		}
		return "Cache hit"

	case "fetch_transport":
		transportType, _ := args["type"].(string)
		if transportType == "" {
			transportType = "all"
		}

		var total int
		if transportType == "bus" || transportType == "all" {
			arrivals := fetchTransportArrivals(agent.Lat, agent.Lon, "NaptanPublicBusCoachTram", "🚌")
			for _, arr := range arrivals {
				Get().Insert(arr)
			}
			total += len(arrivals)
		}
		if transportType == "tube" || transportType == "all" {
			arrivals := fetchTransportArrivals(agent.Lat, agent.Lon, "NaptanMetroStation", "🚇")
			for _, arr := range arrivals {
				Get().Insert(arr)
			}
			total += len(arrivals)
		}
		if transportType == "rail" || transportType == "all" {
			arrivals := fetchTransportArrivals(agent.Lat, agent.Lon, "NaptanRailStation", "🚆")
			for _, arr := range arrivals {
				Get().Insert(arr)
			}
			total += len(arrivals)
		}
		state.LastTransportFetch = time.Now()
		return fmt.Sprintf("Fetched %d arrivals", total)

	case "set_next_cycle":
		minutes, _ := args["minutes"].(float64)
		reason, _ := args["reason"].(string)
		if minutes < 1 {
			minutes = 1
		}
		if minutes > 60 {
			minutes = 60
		}
		state.NextCycle = time.Now().Add(time.Duration(minutes) * time.Minute)
		state.NextCycleReason = reason
		return fmt.Sprintf("Next cycle in %.0fm", minutes)

	case "done":
		summary, _ := args["summary"].(string)
		return summary

	default:
		return fmt.Sprintf("Unknown tool: %s", tool)
	}
}

// Helpers

func formatTimeAgo(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%.0fm ago", d.Minutes())
	}
	return fmt.Sprintf("%.0fh ago", d.Hours())
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// LLM client for agents
var agentLLMClient *openai.Client
var agentModel string

func getAgentLLMClient() *openai.Client {
	if agentLLMClient != nil {
		return agentLLMClient
	}

	// Prefer Fanar
	fanarKey := os.Getenv("FANAR_API_KEY")
	fanarURL := os.Getenv("FANAR_API_URL")
	if fanarKey != "" && fanarURL != "" {
		config := openai.DefaultConfig(fanarKey)
		config.BaseURL = fanarURL
		agentLLMClient = openai.NewClientWithConfig(config)
		agentModel = "Fanar"
		return agentLLMClient
	}

	// Fall back to OpenAI
	key := os.Getenv("OPENAI_API_KEY")
	if key != "" {
		agentLLMClient = openai.NewClient(key)
		agentModel = openai.GPT4oMini
		return agentLLMClient
	}

	return nil
}

func getAgentModel() string {
	getAgentLLMClient()
	return agentModel
}

// AddEvent queues an event for an agent to process in its next cycle
func AddEvent(agentID string, event string) {
	state := GetAgentState(agentID)
	state.Events = append(state.Events, event)
	if len(state.Events) > 10 {
		state.Events = state.Events[len(state.Events)-10:]
	}
}

// SetActiveUsers updates the user count for an agent
func SetActiveUsers(agentID string, count int) {
	state := GetAgentState(agentID)
	state.ActiveUsers = count
}

// AgenticMode controls whether agents use LLM processing
var AgenticMode = false

// EnableAgenticMode turns on LLM-based agent processing
func EnableAgenticMode() {
	AgenticMode = true
	log.Printf("[agent] Agentic mode enabled")
}

// DisableAgenticMode returns to simple polling
func DisableAgenticMode() {
	AgenticMode = false
	log.Printf("[agent] Agentic mode disabled")
}
