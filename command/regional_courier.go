package command

import (
	"fmt"
	"strings"

	"malten.ai/spatial"
)

func init() {
	Register(&Command{
		Name:        "couriers",
		Description: "Regional courier status and control",
		Usage:       "/couriers [on|off|status]",
		Emoji:       "🚴",
		Handler: func(ctx *Context, args []string) (string, error) {
			if len(args) > 0 {
				switch strings.ToLower(args[0]) {
				case "on":
					spatial.EnableRegionalCouriers()
					return "🚴 Regional couriers enabled! Mapping all regions...", nil
				case "off":
					spatial.DisableRegionalCouriers()
					return "🚴 Regional couriers paused.", nil
				}
			}

			// Show status
			status := spatial.GetRegionalCourierStatus()

			enabled := status["enabled"].(bool)
			count := status["courier_count"].(int)
			totalTrips := status["total_trips"].(int)
			totalWalked := status["total_walked"].(float64)

			var sb strings.Builder
			sb.WriteString("🚴 **Regional Couriers**\n\n")

			if enabled {
				sb.WriteString("✅ Active\n")
			} else {
				sb.WriteString("⏸️ Paused\n")
			}

			sb.WriteString(fmt.Sprintf("📊 %d regions · %d trips · %.1f km walked\n\n", count, totalTrips, totalWalked))

			// Show individual couriers
			if couriers, ok := status["couriers"].([]map[string]interface{}); ok && len(couriers) > 0 {
				sb.WriteString("**Regions:**\n")
				for _, c := range couriers {
					statusIcon := "⏸️"
					if c["status"] == "walking" {
						statusIcon = "🚶"
					} else if c["status"] == "active" {
						statusIcon = "✅"
					}

					target := c["target"].(string)
					if target == "" {
						target = "selecting..."
					}

					sb.WriteString(fmt.Sprintf("%s %s → %s (%.0f%% · %d trips)\n",
						statusIcon, c["id"], target, c["progress"], c["trips"]))
				}
			}

			return sb.String(), nil
		},
	})
}
