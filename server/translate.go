package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"time"
)

var languageTag = regexp.MustCompile("^[a-zA-Z]{2,8}(-[a-zA-Z0-9]{1,8}){0,5}$")

// Translate looks up the message in its stream so only posted text is sent to Mu.
func Translate(token string) http.HandlerFunc {
	client := &http.Client{Timeout: 30 * time.Second}
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
		body, _ := json.Marshal(map[string]string{"text": messages[0].Text, "to": input.To})
		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://micro.mu/api/v1/text/translate", bytes.NewReader(body))
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
		var result struct{ Result string }
		if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 512<<10)).Decode(&result) != nil || result.Result == "" {
			http.Error(w, "Translation unavailable", http.StatusBadGateway)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"text": result.Result})
	}
}
