package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/asim/malten/agent"
	"github.com/asim/malten/agent/reflection"
)

// handleSummary saves a stream entry before acknowledging the request. The
// server lifecycle owns generation; disconnecting the browser cannot cancel it.
func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	who := owner(r)
	if who == "" {
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
	sort.Strings(input.IDs)
	raw, _ := json.Marshal(input)
	key := agent.Key(append([]byte(who), raw...))
	b := s.stream
	// Deduplicate before rate limiting, including retries after a lost response.
	b.Lock()
	b.prune(time.Now())
	for _, p := range b.posts {
		if p.Agent == "Summary" && p.key == key && p.owner == who && p.Summary != "failed" {
			b.Unlock()
			writeJSON(w, 202, p)
			return
		}
	}
	b.Unlock()
	if _, err := s.summaryCaptures(input.Stream, input.IDs); err != nil {
		http.Error(w, "These posts have changed. Refresh and try again.", 409)
		return
	}
	if !b.allow(r) {
		w.Header().Set("Retry-After", "60")
		http.Error(w, "Please wait a minute before starting another summary.", 429)
		return
	}
	b.Lock()
	defer b.Unlock()
	// A concurrent retry may have saved the same job while validation ran.
	pending := 0
	for _, p := range b.posts {
		if p.Agent == "Summary" && p.key == key && p.owner == who && p.Summary != "failed" {
			writeJSON(w, 202, p)
			return
		}
		if p.Summary == "pending" {
			pending++
		}
	}
	if pending >= 20 {
		http.Error(w, "Summaries are busy. Your request can retry shortly.", 503)
		return
	}
	p := Post{ID: key, Stream: input.Stream, Text: "Summarising…", Agent: "Summary", Summary: "pending", Created: time.Now().UnixMilli(), owner: who, key: key, summaryIDs: input.IDs}
	before := append([]Post(nil), b.posts...)
	replaced := false
	for i, old := range b.posts {
		if old.ID == key {
			b.posts[i] = p
			replaced = true
			break
		}
	}
	if !replaced {
		if len(b.posts) >= capacity {
			b.posts = b.posts[1:]
		}
		b.posts = append(b.posts, p)
	}
	if err := b.save(); err != nil {
		b.posts = before
		http.Error(w, "Could not save summary request.", 503)
		return
	}
	writeJSON(w, 202, p)
}

// Keep queued work responsive even while reported posts are being reviewed.
func (s *Server) runSummaries(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	defer s.summaryWorkers.Wait()
	for {
		if ctx.Err() != nil {
			return
		}
		s.startSummaries(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Pending entries survive restarts in the same snapshot as their captures.
func (s *Server) startSummaries(ctx context.Context) {
	b := s.stream
	b.Lock()
	defer b.Unlock()
	b.prune(time.Now())
	for i := range b.posts {
		p := &b.posts[i]
		if p.Summary != "pending" || p.summaryRunning || p.hidden {
			continue
		}
		select {
		case s.summarySlots <- struct{}{}:
		default:
			return
		}
		p.summaryRunning = true
		job := *p
		s.summaryWorkers.Add(1)
		go func() { defer s.summaryWorkers.Done(); defer func() { <-s.summarySlots }(); s.runSummary(ctx, job) }()
	}
}

func (s *Server) runSummary(parent context.Context, job Post) {
	ctx, cancel := context.WithTimeout(parent, 150*time.Second)
	defer cancel()
	captures, err := s.summaryCaptures(job.Stream, job.summaryIDs)
	text := ""
	if err == nil {
		var result reflection.Result
		result, err = s.summarise(ctx, captures)
		if err == nil {
			text = result.Summary
			for _, note := range result.Context {
				text += "\n\n" + note.Text
				for _, source := range note.Sources {
					title := strings.NewReplacer("[", "", "]", "", "\n", " ").Replace(source.Title)
					text += "\n[" + title + "](" + source.URL + ")"
				}
			}
			if len(result.Unavailable) > 0 {
				text += "\n\nSome sources were unavailable."
			}
			candidate := job
			candidate.Text = text
			var allowed bool
			allowed, err = s.stream.moderate(ctx, candidate)
			if err == nil && !allowed {
				err = errors.New("summary not suitable for sharing")
			}
		}
	}
	b := s.stream
	b.Lock()
	defer b.Unlock()
	for i := range b.posts {
		p := &b.posts[i]
		if p.ID != job.ID {
			continue
		}
		before := *p
		p.summaryRunning = false
		if parent.Err() != nil {
			return
		} // Leave pending for the next server startup.
		// Validate again under the commit lock, after moderation too.
		if _, e := s.summaryCapturesLocked(job.Stream, job.summaryIDs); e != nil {
			err = e
		}
		if err != nil {
			p.Summary = "failed"
			p.Text = "Could not summarise these posts. Tap Summarise to try again."
			log.Printf("summary: %v", err)
		} else {
			p.Summary = "done"
			p.Text = text
		}
		if e := b.save(); e != nil {
			*p = before
			p.summaryRunning = false
			log.Printf("save summary: %v", e)
		}
		return
	}
}

func (s *Server) summaryCaptures(stream string, ids []string) ([]agent.Observation, error) {
	b := s.stream
	b.Lock()
	defer b.Unlock()
	b.prune(time.Now())
	return s.summaryCapturesLocked(stream, ids)
}

func (s *Server) summaryCapturesLocked(stream string, ids []string) ([]agent.Observation, error) {
	b := s.stream
	wanted := map[string]bool{}
	for _, id := range ids {
		if id == "" || wanted[id] {
			return nil, errors.New("invalid capture IDs")
		}
		wanted[id] = true
	}
	var out []agent.Observation
	size := 0
	for _, p := range b.posts {
		if wanted[p.ID] && p.Stream == stream && !p.hidden && p.Agent == "" && time.Since(time.UnixMilli(p.Created)) < lifetime {
			size += len([]rune(p.Text))
			if size > 60000 {
				return nil, errors.New("too much summary context")
			}
			out = append(out, agent.Observation{ID: p.ID, Stream: p.Stream, Text: p.Text, Photo: p.Photo, Kind: "human", At: time.UnixMilli(p.Created)})
		}
	}
	if len(out) != len(wanted) {
		return nil, errors.New("captures unavailable")
	}
	return out, nil
}
