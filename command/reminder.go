package command

import (
	"encoding/json"

	"malten.ai/spatial"
)

func init() {
	Register(&Command{
		Name:        "reminder",
		Description: "Daily verse and reminder",
		Handler: func(ctx *Context, args []string) (string, error) {
			r := spatial.GetDailyReminder()
			if r == nil {
				return "Reminder unavailable", nil
			}
			
			// Return as JSON for client to render as card
			data, _ := json.Marshal(r)
			return string(data), nil
		},
	})
}
