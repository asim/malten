package command

import (
	"encoding/json"
	"strings"

	"malten.ai/spatial"
)

func init() {
	Register(&Command{
		Name:        "reminder",
		Description: "Daily verse and reminder",
		Usage:       "/reminder [type]",
		Emoji:       "💿",
		LoadingText: "Getting reminder...",
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
				return `{"error": "Reminder unavailable"}`, nil
			}
			
			// Return JSON for client
			return formatReminderJSON(r), nil
		},
	})
}

// ReminderResponse is the JSON format for client
type ReminderResponse struct {
	Verse      string `json:"verse,omitempty"`
	Name       string `json:"name,omitempty"`
	NameNumber int    `json:"name_number,omitempty"`
	Message    string `json:"message,omitempty"`
}

func formatReminderJSON(r *spatial.Reminder) string {
	resp := ReminderResponse{
		Verse:      r.Verse,
		Name:       r.Name,
		NameNumber: r.GetNameNumber(),
		Message:    r.Message,
	}
	b, _ := json.Marshal(resp)
	return string(b)
}
