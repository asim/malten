package command

import (
	"fmt"
	"strings"
	"time"

	"malten.ai/spatial"
)

func init() {
	Register(&Command{
		Name:        "agents",
		Description: "Show status of all agents",
		Usage:       "/agents",
		Handler: func(ctx *Context, args []string) (string, error) {
			return Agents(), nil
		},
	})
}

// Agents shows status of all agents
func Agents() string {
	db := spatial.Get()
	agents := db.ListAgents()

	if len(agents) == 0 {
		return "🤖 No agents running"
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("🤖 %d agents", len(agents)))
	lines = append(lines, "")

	for _, agent := range agents {
		status := "idle"
		if agentData := agent.GetAgentData(); agentData != nil && agentData.Status != "" {
			status = agentData.Status
		}

		// Get last update time
		lastUpdate := agent.UpdatedAt
		ago := time.Since(lastUpdate)
		agoStr := formatDuration(ago)

		// Count entities this agent has indexed
		agentID := agent.ID
		places := countEntitiesByAgent(db, agentID, spatial.EntityPlace)
		arrivals := countEntitiesByAgent(db, agentID, spatial.EntityArrival)

		line := fmt.Sprintf("%s - %s %s", agent.Name, status, agoStr)
		if places > 0 || arrivals > 0 {
			line += fmt.Sprintf("\n  📍 %d places, 🚏 %d stops", places, arrivals)
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

func countEntitiesByAgent(db *spatial.DB, agentID string, entityType spatial.EntityType) int {
	// Count entities that have this agent_id in their data
	return db.CountByAgentID(agentID, entityType)
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return "now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}
