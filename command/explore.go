package command

import (
	"fmt"
	"strings"

	"malten.ai/spatial"
)

func init() {
	Register(&Command{
		Name:        "explore",
		Description: "Toggle agent exploration mode",
		Usage:       "/explore [on|off|status]",
		Handler:     handleExplore,
	})
}

func handleExplore(ctx *Context, args []string) (string, error) {
	if len(args) == 0 {
		// Show status
		status := "off"
		if spatial.ExplorationMode {
			status = "on"
		}
		stats := spatial.GetExplorerStats()
		return fmt.Sprintf("🧭 Exploration mode: %s\n\nExploring agents: %d\nTotal exploration steps: %d\n\nUse /explore on to enable, /explore off to disable",
			status, stats["exploring_agents"], stats["total_steps"]), nil
	}

	switch strings.ToLower(args[0]) {
	case "on":
		spatial.ExplorationMode = true
		return "🧭 Exploration mode enabled. Agents will now move and map streets.", nil
	case "off":
		spatial.ExplorationMode = false
		return "🧭 Exploration mode disabled. Agents will stay stationary.", nil
	case "status":
		status := "off"
		if spatial.ExplorationMode {
			status = "on"
		}
		stats := spatial.GetExplorerStats()
		
		// List exploring agents
		var lines []string
		lines = append(lines, fmt.Sprintf("🧭 Exploration mode: %s", status))
		lines = append(lines, "")
		
		db := spatial.Get()
		agents := db.ListAgents()
		for _, agent := range agents {
			steps, _, _, dist := spatial.GetAgentExplorationStats(agent)
			if steps > 0 {
				lines = append(lines, fmt.Sprintf("• %s: %d steps, %.0fm from home", 
					agent.Name, steps, dist))
			}
		}
		
		if len(lines) == 2 {
			lines = append(lines, "No agents have explored yet.")
		}
		
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("Total steps: %d", stats["total_steps"]))
		
		return strings.Join(lines, "\n"), nil
	default:
		return "Usage: /explore [on|off|status]", nil
	}
}
