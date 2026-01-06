package command

import (
	"fmt"
	"strings"

	"malten.ai/spatial"
)

func init() {
	Register(&Command{
		Name:        "courier",
		Description: "Control the courier agent that connects areas",
		Usage:       "/courier [on|off|status]",
		Handler:     handleCourier,
	})
}

func handleCourier(ctx *Context, args []string) (string, error) {
	if len(args) == 0 {
		return courierStatus(), nil
	}

	switch strings.ToLower(args[0]) {
	case "on":
		spatial.EnableCourier()
		return "🚴 Courier enabled! Connecting areas...", nil

	case "off":
		spatial.DisableCourier()
		return "⏸️ Courier paused", nil

	case "status":
		return courierStatus(), nil

	default:
		return courierStatus(), nil
	}
}

func courierStatus() string {
	stats := spatial.GetCourierStats()

	if !stats["initialized"].(bool) {
		return "🚴 Courier not initialized\n\nUse `/courier on` to start"
	}

	var sb strings.Builder
	sb.WriteString("🚴 **Courier Status**\n\n")

	if stats["enabled"].(bool) {
		sb.WriteString("✅ Active\n")
	} else {
		sb.WriteString("⏸️ Paused\n")
	}

	if headingTo, ok := stats["heading_to"].(string); ok && headingTo != "" {
		sb.WriteString(fmt.Sprintf("🎯 Heading to: %s\n", headingTo))
		if progress, ok := stats["progress"].(float64); ok {
			sb.WriteString(fmt.Sprintf("📍 Progress: %.0f%%\n", progress))
		}
	}

	sb.WriteString(fmt.Sprintf("\n📊 Stats:\n"))
	sb.WriteString(fmt.Sprintf("• Trips complete: %v\n", stats["trips_complete"]))
	sb.WriteString(fmt.Sprintf("• Distance walked: %.1f km\n", stats["km_walked"]))

	return sb.String()
}
