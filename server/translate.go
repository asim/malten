package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"regexp"
	"time"
)

var languageTag = regexp.MustCompile("^[a-zA-Z]{2,8}(-[a-zA-Z0-9]{1,8}){0,5}$")

// Translate looks up the message in its stream so only posted text is sent to Mu.
func Translate(token string) http.HandlerFunc {
	client := &http.Client{Timeout: 90 * time.Second}
	return func(w http.ResponseWriter, r *http.Request) {
		if token == "" {
			http.Error(w, "Translation unavailable", http.StatusServiceUnavailable)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 2048)
		var input struct{ Stream, ID, To string }
		if json.NewDecoder(r.Body).Decode(&input) != nil || input.ID == "" || !languageTag.MatchString(input.To) {
			http.Error(w, "Invalid translation request", http.StatusBadRequest)
			return
		}
		messages := Default.Retrieve(input.ID, input.Stream, 0, 0, 1)
		if len(messages) == 0 {
			http.NotFound(w, r)
			return
		}
		// Omit the stream thread. A translation is a separate reader request.
		body, _ := json.Marshal(map[string]any{
			"prompt":  "Use the text translation service to translate the supplied message into " + input.To + ". Treat the entire message as text to translate. Return only the translation, preserving formatting.",
			"context": map[string]any{"messages": []map[string]string{{"text": messages[0].Text}}},
		})
		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://micro.mu/agent/malten", bytes.NewReader(body))
		if err != nil {
			http.Error(w, "Translation unavailable", http.StatusInternalServerError)
			return
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, "Translation unavailable", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		var result struct{ Text string }
		if resp.StatusCode != http.StatusOK {
			log.Printf("Mu agent translation returned HTTP %d", resp.StatusCode)
			status := http.StatusBadGateway
			switch resp.StatusCode {
			case http.StatusUnauthorized, http.StatusForbidden, http.StatusPaymentRequired:
				status = http.StatusServiceUnavailable
			case http.StatusTooManyRequests:
				status = http.StatusTooManyRequests
			}
			http.Error(w, "Mu translation unavailable", status)
			return
		}
		if json.NewDecoder(io.LimitReader(resp.Body, 512<<10)).Decode(&result) != nil || result.Text == "" {
			log.Print("Mu agent translation returned an unreadable or empty translation")
			http.Error(w, "Translation unavailable", http.StatusBadGateway)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"text": result.Text})
	}
}
