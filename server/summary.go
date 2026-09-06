package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/asim/malten/agent"
)

// Summary is returned only to the requesting reader. It is never a public post,
// an agent observation, or a persisted extension of the captures' lifetime.
func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	if owner(r) == "" {
		http.Error(w, "missing browser identity", 401)
		return
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "JSON required", 415)
		return
	}
	var input struct {
		Stream string
		IDs    []string
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	d.DisallowUnknownFields()
	if d.Decode(&input) != nil || d.Decode(new(any)) != io.EOF || input.Stream == "" || !validStream(input.Stream) || len(input.IDs) < 1 || len(input.IDs) > 40 {
		http.Error(w, "invalid summary request", 400)
		return
	}
	captures, err := s.summaryCaptures(input.Stream, input.IDs)
	if err != nil {
		http.Error(w, "These posts have changed. Refresh and try again.", 409)
		return
	}
	select {
	case s.summarySlots <- struct{}{}:
		defer func() { <-s.summarySlots }()
	default:
		http.Error(w, "Summaries are busy. Try again shortly.", 503)
		return
	}
	if !s.stream.allow(r) {
		w.Header().Set("Retry-After", "60")
		http.Error(w, "Please wait a minute before summarising again.", 429)
		return
	}
	// Retrieval can take more than one model call; leave posting's timeout alone.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(90 * time.Second))
	ctx, cancel := context.WithTimeout(r.Context(), 80*time.Second)
	defer cancel()
	result, err := s.summarise(ctx, captures)
	if err != nil {
		http.Error(w, "Could not summarise just now. Please try again.", 503)
		return
	}
	// A report, deletion or expiry during generation invalidates the result.
	if _, err = s.summaryCaptures(input.Stream, input.IDs); err != nil {
		http.Error(w, "These posts have changed. Refresh and try again.", 409)
		return
	}
	writeJSON(w, 200, result)
}

func (s *Server) summaryCaptures(stream string, ids []string) ([]agent.Observation, error) {
	b := s.stream
	b.Lock()
	defer b.Unlock()
	b.prune(time.Now())
	wanted := map[string]bool{}
	for _, id := range ids {
		if id == "" || wanted[id] {
			return nil, errors.New("invalid capture IDs")
		}
		wanted[id] = true
	}
	var out []agent.Observation
	for _, p := range b.posts {
		if wanted[p.ID] && p.Stream == stream && !p.hidden && p.Agent == "" {
			out = append(out, agent.Observation{ID: p.ID, Stream: p.Stream, Text: p.Text, Photo: p.Photo, Kind: "human", At: time.UnixMilli(p.Created)})
		}
	}
	if len(out) != len(wanted) {
		return nil, errors.New("captures unavailable")
	}
	return out, nil
}
