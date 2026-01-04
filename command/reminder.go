package command

import (
	"encoding/json"
	"strings"

	"malten.ai/spatial"
)

func init() {
	Register(&Command{
		Name:        "reminder",
		Description: "Daily verse and reminder. Usage: /reminder [type]",
		Handler: func(ctx *Context, args []string) (string, error) {
			var r *spatial.Reminder
			
			// Check for specific reminder type
			if len(args) > 0 {
				key := strings.ToLower(args[0])
				// Try time-based reminder first
				r = spatial.GetTimeReminder(key)
				if r == nil {
					// Fall back to daily
					r = spatial.GetDailyReminder()
				}
			} else {
				r = spatial.GetDailyReminder()
			}
			
			if r == nil {
				return "Reminder unavailable", nil
			}
			
			// Return as JSON for client to render as card
			data, _ := json.Marshal(r)
			return string(data), nil
		},
	})
}
