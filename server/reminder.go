package server

import (
	"encoding/json"
	"net/http"

	"malten.ai/spatial"
)

// ReminderHandler returns today's daily reminder
func ReminderHandler(w http.ResponseWriter, r *http.Request) {
	reminder := spatial.GetDailyReminder()
	if reminder == nil {
		http.Error(w, "reminder unavailable", 503)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reminder)
}
