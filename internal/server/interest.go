package server

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// interest is one waitlist signup from someone outside OS map coverage. This is
// the one thing the server persists — opt-in contact details someone chooses to
// leave, not private user content. It is appended to a JSONL file.
type interest struct {
	ID        string    `json:"id"`
	Place     string    `json:"place,omitempty"` // the city/town someone's asking for
	Email     string    `json:"email,omitempty"`
	Note      string    `json:"note,omitempty"`
	Country   string    `json:"country,omitempty"`
	Lat       float64   `json:"lat,omitempty"`
	Lng       float64   `json:"lng,omitempty"`
	IP        string    `json:"ip,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// interestBook is an append-only, file-backed list of signups.
type interestBook struct {
	mu   sync.Mutex
	path string
}

func (b *interestBook) add(i interest) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	f, err := os.OpenFile(b.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	line, err := json.Marshal(i)
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	return err
}

func (b *interestBook) all() ([]interest, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	f, err := os.Open(b.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []interest{}, nil
		}
		return nil, err
	}
	defer f.Close()
	out := []interest{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		var i interest
		if json.Unmarshal(sc.Bytes(), &i) == nil {
			out = append(out, i)
		}
	}
	return out, sc.Err()
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n]
	}
	return s
}

func looksLikeEmail(s string) bool {
	at := strings.IndexByte(s, '@')
	return at > 0 && at < len(s)-3 && strings.IndexByte(s[at:], '.') > 0 && !strings.ContainsAny(s, " \t\n")
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	host := r.RemoteAddr
	if i := strings.LastIndexByte(host, ':'); i > 0 {
		host = host[:i]
	}
	return host
}

// handleInterest records a signup (POST) or, for an operator holding the admin
// token, returns the whole list (GET). Without a configured admin token the GET
// is disabled, so signups are never public.
func (s *Server) handleInterest(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var in struct {
			Place   string  `json:"place"`
			Email   string  `json:"email"`
			Note    string  `json:"note"`
			Country string  `json:"country"`
			Lat     float64 `json:"lat"`
			Lng     float64 `json:"lng"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
			return
		}
		// A city request needs a place OR an email; the email is optional here.
		place := clip(in.Place, 120)
		email := clip(in.Email, 200)
		if email != "" && !looksLikeEmail(email) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "that email doesn't look right"})
			return
		}
		if place == "" && email == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tell us your city (an email is optional)"})
			return
		}
		rec := interest{
			ID: newID(), Place: place, Email: email, Note: clip(in.Note, 500), Country: clip(in.Country, 80),
			Lat: in.Lat, Lng: in.Lng, IP: clientIP(r), CreatedAt: time.Now().UTC(),
		}
		if err := s.interest.add(rec); err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	case http.MethodGet:
		if s.adminToken == "" || r.URL.Query().Get("token") != s.adminToken {
			http.NotFound(w, r)
			return
		}
		list, err := s.interest.all()
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"count": len(list), "signups": list})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// fail logs the real error and returns a generic message.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "something went wrong"})
	_ = err
}

// newID returns a short random id for a signup.
func newID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	const hex = "0123456789abcdef"
	out := make([]byte, 12)
	for i, x := range b {
		out[2*i] = hex[x>>4]
		out[2*i+1] = hex[x&0xf]
	}
	return "I-" + string(out)
}
