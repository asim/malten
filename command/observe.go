package command

import (
	"fmt"
	"strings"

	"malten.ai/spatial"
)

func init() {
	Register(&Command{
		Name:        "observe",
		Description: "Show pending observations and trigger awareness processing",
		Usage:       "/observe",
		Handler:     handleAwareness,
	})
}

func handleAwareness(ctx *Context, args []string) (string, error) {
	if !ctx.HasLocation() {
		return "📍 No location. Enable location first.", nil
	}

	db := spatial.Get()
	agent := db.FindAgent(ctx.Lat, ctx.Lon, spatial.AgentRadius)
	if agent == nil {
		return "No agent for your area.", nil
	}

	log := spatial.GetObservationLog()

	// Process if requested
	if len(args) > 0 && args[0] == "process" {
		items, err := spatial.ProcessAwareness(agent.ID, agent.Name, map[string]interface{}{
			"lat": ctx.Lat,
			"lon": ctx.Lon,
		})
		if err != nil {
			return fmt.Sprintf("❌ Processing error: %v", err), nil
		}
		if len(items) == 0 {
			return "✓ Processed - nothing worth surfacing right now.", nil
		}

		var result []string
		result = append(result, fmt.Sprintf("🔔 %d items to surface:\n", len(items)))
		for _, item := range items {
			result = append(result, fmt.Sprintf("%s %s", item.Emoji, item.Message))
		}
		return strings.Join(result, "\n"), nil
	}

	// Show pending observations
	pending := log.GetPending(agent.ID)
	if len(pending) == 0 {
		return fmt.Sprintf("👁️ **Awareness for %s**\n\nNo pending observations.\n\nUse `/observe process` to run awareness filter.", agent.Name), nil
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("👁️ **Awareness for %s**\n", agent.Name))
	lines = append(lines, fmt.Sprintf("%d pending observations:\n", len(pending)))

	for _, obs := range pending {
		lines = append(lines, fmt.Sprintf("• %s %s: %v",
			obs.Time.Format("15:04"), obs.Type, obs.Data))
	}

	lines = append(lines, "\nUse `/observe process` to run awareness filter.")
	return strings.Join(lines, "\n"), nil
}

func init() {
	Register(&Command{
		Name:        "trigger",
		Description: "Add test observations for debugging",
		Usage:       "/test-awareness",
		Handler:     handleTestAwareness,
	})
}

func handleTestAwareness(ctx *Context, args []string) (string, error) {
	if !ctx.HasLocation() {
		return "📍 No location.", nil
	}

	db := spatial.Get()
	agent := db.FindAgent(ctx.Lat, ctx.Lon, spatial.AgentRadius)
	if agent == nil {
		return "No agent.", nil
	}

	// Add some test observations
	spatial.AddWeatherObservation(agent.ID, agent.Name, 5.0, "☁️ 5°C", "🌧️ Rain at 15:00 (70%)")
	spatial.AddDisruptionObservation(agent.ID, agent.Name, "Severe", "A316", "Road closure due to accident")
	spatial.AddNewPlaceObservation(agent.ID, agent.Name, "Grind & Steam", "cafe", "72 Milton Road")

	return "✓ Added 3 test observations. Run `/observe` to see them.", nil
}
