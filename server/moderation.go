package server

import (
	"crypto/sha256"
	"crypto/subtle"
	_ "embed"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"sort"
	"time"
)

const removedMessage = "removed"

// Removed lets a publisher respect a moderator's decision until the post expires.
func (s *Server) Removed(id, stream string) bool {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	m := s.streams[stream].record(id, time.Now().UnixNano())
	return m != nil && m.Type == removedMessage
}

func (s *Server) reviewRevision(stream string) uint64 {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	if st := s.streams[stream]; st != nil {
		return st.ReviewRevision
	}
	return 0
}

//go:embed moderation.html
var moderationHTML string
var moderationPage = template.Must(template.New("moderation").Parse(moderationHTML))

type reportInput struct {
	Stream, ID, Reason string
}

type reportEntry struct {
	Stream, ID, Reason, Text, Link string
	Posted                         time.Time
}

func (s *Server) reports() []reportEntry {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	var entries []reportEntry
	now := time.Now().UnixNano()
	for _, st := range s.streams {
		if st.Private {
			continue
		}
		for _, m := range st.Messages {
			if m.Review == "" || m.Review == "dismissed" || now-m.Created >= st.TTL {
				continue
			}
			link := url.URL{Path: "/", RawQuery: url.Values{"message": {m.Id}}.Encode()}
			if st.Id != DefaultStream {
				link.Fragment = st.Id
			}
			entries = append(entries, reportEntry{st.Id, m.Id, m.Review, m.Text, link.String(), time.Unix(0, m.Created).UTC()})
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Posted.After(entries[j].Posted) })
	return entries
}

// Store the review on the original. A removed post retains only its ID and
// lifetime so a retry cannot republish it. Replies contain no stored excerpts.
func (s *Server) review(in reportInput, action string) bool {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	st := s.streams[in.Stream]
	if st == nil || st.Private {
		return false
	}
	now := time.Now().UnixNano()
	for i, m := range st.Messages {
		if m.Id != in.ID || now-m.Created >= st.TTL {
			continue
		}
		if m.Type == removedMessage {
			return action == "remove"
		}
		next := *m
		switch action {
		case "report":
			if m.Review != "" {
				return true
			}
			next.Review = in.Reason
		case "dismiss", "remove":
			if m.Review == "dismissed" {
				return action == "dismiss"
			}
			if m.Review == "" {
				return false
			}
			if action == "dismiss" {
				next.Review = "dismissed"
			} else {
				next = Message{Id: m.Id, Stream: m.Stream, Created: m.Created, Type: removedMessage}
				delete(s.metadata, m.Id)
				st.ReviewRevision++
			}
		default:
			return false
		}
		st.Messages[i] = &next
		s.dirty = true
		return true
	}
	return false
}

// ModerationHandler serves public reporting and a password-protected review queue.
func (s *Server) ModerationHandler(token string) http.Handler {
	secret := sha256.Sum256([]byte(token))
	mux := http.NewServeMux()
	apply := func(w http.ResponseWriter, in reportInput, action string) bool {
		if !s.review(in, action) {
			http.Error(w, "This reflection is no longer available for review.", http.StatusNotFound)
			return false
		}
		if action == "remove" {
			event := NewEvent("removed", in.Stream)
			event.Parent = in.ID
			s.Broadcast(event)
		}
		if err := s.Checkpoint(); err != nil {
			log.Printf("save moderation decision: %v", err)
			http.Error(w, "Could not save the change. Please try again.", http.StatusServiceUnavailable)
			return false
		}
		return true
	}
	mux.HandleFunc("POST /reports", func(w http.ResponseWriter, r *http.Request) {
		var in reportInput
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
			http.Error(w, "Invalid report.", http.StatusBadRequest)
			return
		}
		switch in.Reason {
		case "Abuse or harassment", "Spam", "Other":
		default:
			http.Error(w, "Choose a reason for the report.", http.StatusBadRequest)
			return
		}
		if err := s.limitWrites("report", ClientNetwork(r), 5); err != nil {
			WriteQuotaError(w, err)
			return
		}
		if apply(w, in, "report") {
			w.WriteHeader(http.StatusNoContent)
		}
	})
	mux.HandleFunc("/moderation", func(w http.ResponseWriter, r *http.Request) {
		if token == "" {
			http.NotFound(w, r)
			return
		}
		user, pass, ok := r.BasicAuth()
		key := sha256.Sum256([]byte(pass))
		if !ok || user != "moderator" || subtle.ConstantTimeCompare(key[:], secret[:]) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="Malten moderation", charset="UTF-8"`)
			http.Error(w, "Moderator sign-in required.", http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if err := moderationPage.Execute(w, s.reports()); err != nil {
				log.Printf("render moderation queue: %v", err)
			}
		case http.MethodPost:
			r.Body = http.MaxBytesReader(w, r.Body, 4096)
			if r.ParseForm() != nil {
				http.Error(w, "Invalid decision.", http.StatusBadRequest)
				return
			}
			action := r.PostForm.Get("action")
			if action != "remove" && action != "dismiss" {
				http.Error(w, "Choose a moderation action.", http.StatusBadRequest)
				return
			}
			in := reportInput{Stream: r.PostForm.Get("stream"), ID: r.PostForm.Get("id")}
			if apply(w, in, action) {
				http.Redirect(w, r, "/moderation", http.StatusSeeOther)
			}
		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, "Method unavailable.", http.StatusMethodNotAllowed)
		}
	})
	protected := http.NewCrossOriginProtection().Handler(mux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		protected.ServeHTTP(w, r)
	})
}
