package agent

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
)

var languageTag = regexp.MustCompile("^[a-zA-Z]{2,8}(-[a-zA-Z0-9]{1,8}){0,5}$")

// Translate sends one selected message to the translation agent in a fresh request.
func (a *Agent) Translate(w http.ResponseWriter, r *http.Request) {
	if a.token == "" {
		http.Error(w, "Translation unavailable", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	var input struct{ Stream, ID, To string }
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.ID == "" || !languageTag.MatchString(input.To) {
		http.Error(w, "Invalid translation request", http.StatusBadRequest)
		return
	}
	messages := a.server.Retrieve(input.ID, input.Stream, 0, 0, 1)
	if len(messages) == 0 {
		http.NotFound(w, r)
		return
	}
	req := muRequest{Prompt: "Translate the supplied message into " + input.To + "."}
	req.Context.Messages = []contextMessage{{Text: messages[0].Text}}
	reply, err := a.ask(r.Context(), a.baseURL+"/translate", req)
	if err != nil {
		log.Printf("Mu agent translation: %v", err)
		status := http.StatusBadGateway
		var upstream muStatusError
		if errors.As(err, &upstream) {
			switch upstream {
			case http.StatusUnauthorized, http.StatusForbidden, http.StatusPaymentRequired:
				status = http.StatusServiceUnavailable
			case http.StatusTooManyRequests:
				status = http.StatusTooManyRequests
			}
		}
		http.Error(w, "Translation unavailable", status)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"text": reply.Text})
}
