package session

import (
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"mime"
	"net"
	"net/http"
	"sort"
	"strconv"
)

type clientError struct {
	code int
	text string
}

func (e clientError) Error() string { return e.text }

var (
	errMissing  = clientError{404, "This booking is unavailable in this browser."}
	errHost     = clientError{403, "Only the host can do that."}
	errInactive = clientError{409, "This private reflection is not active."}
	errInvite   = clientError{409, "This invite is unavailable or has already been claimed."}
	errReply    = clientError{502, "Your question was posted. Malten couldn’t reply."}
)

const cookieName = "malten_guest"

func visitor(r *http.Request) string {
	c, err := r.Cookie(cookieName)
	if err != nil || len(c.Value) < 26 || len(c.Value) > 128 {
		return ""
	}
	digest := sha256.Sum256([]byte(c.Value))
	return hex.EncodeToString(digest[:])
}

func setVisitor(w http.ResponseWriter, r *http.Request) {
	if visitor(r) != "" {
		return
	}
	host := r.URL.Hostname()
	if host == "" {
		host = r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
	}
	local := host == "localhost" || net.ParseIP(host).IsLoopback()
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: rand.Text(), Path: "/session", HttpOnly: true,
		Secure: !local || r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https", SameSite: http.SameSiteLaxMode, MaxAge: 365 * 24 * 3600})
}

type view struct {
	Revision     uint64    `json:"revision"`
	Room         string    `json:"room"`
	Now          int64     `json:"now"`
	Status       string    `json:"status"`
	Host         bool      `json:"host"`
	Participants int       `json:"participants"`
	Amount       int       `json:"amount,omitempty"`
	Currency     string    `json:"currency,omitempty"`
	Minutes      int       `json:"minutes,omitempty"`
	EndsAt       int64     `json:"ends_at,omitempty"`
	Checkout     string    `json:"checkout,omitempty"`
	Invites      []string  `json:"invites,omitempty"`
	Messages     []Message `json:"messages,omitempty"`
}

// Called while holding s.mu. Browser responses omit all member credentials and Mu thread IDs.
func (s *Store) view(r *room, who string, detail bool) view {
	p := r.Receipt
	now := s.now().Unix()
	v := view{
		Room: r.Reference, Revision: r.Revision, Now: now, Status: r.status(now),
		Host: r.Host == who, Participants: r.Participants, Minutes: p.Minutes, EndsAt: p.EndsAt,
	}
	if detail && v.Host {
		v.Amount, v.Currency = p.Amount, p.Currency
		switch v.Status {
		case "pending":
			v.Checkout = p.CheckoutURL
		case "paid", "active":
			for _, seat := range r.Seats {
				v.Invites = append(v.Invites, seat.Invite)
			}
		}
	}
	if detail && v.Status == "active" {
		v.Messages = r.Messages // Messages are copy-on-write.
	}
	return v
}

type input struct {
	Parent       string `json:"parent"`
	Payment      string `json:"payment"`
	Room         string `json:"room"`
	Participants int    `json:"participants"`
	Invite       string `json:"invite"`
	Text         string `json:"text"`
	Ask          bool   `json:"ask"`
}

func (s *Store) state(who, ref string) (any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ref != "" {
		r := s.rooms[ref]
		if r != nil {
			if _, ok := r.member(who); ok {
				return s.view(r, who, true), nil
			}
		}
		return nil, errMissing
	}
	var rooms []view
	for _, r := range s.rooms {
		if _, ok := r.member(who); ok && r.status(s.now().Unix()) != "expired" {
			rooms = append(rooms, s.view(r, who, false))
		}
	}
	sort.Slice(rooms, func(i, j int) bool { return s.rooms[rooms[i].Room].Created > s.rooms[rooms[j].Room].Created })
	return struct {
		Available bool   `json:"available"`
		Rooms     []view `json:"rooms"`
	}{s.muPay.Available(), rooms}, nil
}

func (s *Store) action(r *http.Request, who string, in input) (any, error) {
	ctx := r.Context()
	if r.URL.Path == "/session/return" {
		return s.checkoutReturn(ctx, who, in.Payment)
	}
	if r.URL.Path == "/session/book" {
		return s.book(ctx, who, in)
	}
	s.mu.Lock()
	room := s.rooms[in.Room]
	member, host := false, false
	if room != nil {
		_, member = room.member(who)
		host = room.Host == who
	}
	s.mu.Unlock()
	if room == nil || (!member && r.URL.Path != "/session/claim") {
		return nil, errMissing
	}
	var err error
	switch r.URL.Path {
	case "/session/refresh", "/session/start":
		if !host {
			return nil, errHost
		}
		err = s.syncPayment(ctx, room, r.URL.Path == "/session/start")
	case "/session/claim":
		err = s.claim(room, who, in.Invite)
	case "/session/post":
		err = s.post(ctx, room, who, in)
	default:
		return nil, errMissing
	}
	if err != nil {
		return nil, err
	}
	return s.state(who, in.Room)
}

//go:embed web/*
var web embed.FS

func (s *Store) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /session", func(w http.ResponseWriter, r *http.Request) {
		setVisitor(w, r)
		http.ServeFileFS(w, r, web, "web/session.html")
	})
	api := func(w http.ResponseWriter, r *http.Request) {
		who := visitor(r)
		if who == "" {
			http.Error(w, "Open the booking page and allow cookies in this browser.", http.StatusUnauthorized)
			return
		}
		var data any
		var err error
		if r.Method == http.MethodGet {
			data, err = s.state(who, r.URL.Query().Get("room"))
		} else {
			typ, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if typ != "application/json" {
				http.Error(w, "Send JSON.", 415)
				return
			}
			var in input
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&in); err != nil {
				http.Error(w, "Invalid request.", 400)
				return
			}
			data, err = s.action(r, who, in)
		}
		if err != nil {
			if errors.Is(err, errReply) {
				w.Header().Set("X-Malten-Posted", "1")
			}
			var e clientError
			if errors.As(err, &e) {
				http.Error(w, e.text, e.code)
				return
			}
			log.Printf("session request: %v", err)
			http.Error(w, "Couldn’t update this private reflection. Please try again.", 502)
			return
		}
		if v, ok := data.(view); ok && r.Method == http.MethodGet && v.Status != "expired" &&
			r.URL.Query().Get("since") == strconv.FormatUint(v.Revision, 10) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(data)
	}
	for _, file := range []string{"session.css", "session.js"} {
		mux.HandleFunc("GET /session/web/"+file, func(w http.ResponseWriter, r *http.Request) { http.ServeFileFS(w, r, web, "web/"+file) })
	}
	mux.HandleFunc("GET /session/state", api)
	for _, action := range []string{"book", "return", "refresh", "start", "claim", "post"} {
		mux.HandleFunc("POST /session/"+action, api)
	}
	protected := http.NewCrossOriginProtection().Handler(mux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		protected.ServeHTTP(w, r)
	})
}
