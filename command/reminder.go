package command

import (
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
				return "Reminder unavailable", nil
			}
			
			// Return formatted text
			return formatReminder(r), nil
		},
	})
}

func formatReminder(r *spatial.Reminder) string {
	var parts []string
	
	// Name of Allah or verse
	if r.Name != "" {
		parts = append(parts, "💿 "+r.Name)
	}
	if r.Verse != "" {
		parts = append(parts, r.Verse)
	}
	if r.Message != "" {
		parts = append(parts, r.Message)
	}
	
	return strings.Join(parts, "\n")
}
