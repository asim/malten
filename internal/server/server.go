// Package server exposes the agent over HTTP and serves the embedded chat UI.
//
// The server is stateless and anonymous: it keeps no accounts, no sessions and
// no user content. Each request carries the context it needs — the prior
// messages and the user's open issues — and the server assembles a prompt,
// calls the model, streams a reply (plus any issue changes for the client to
// save), and forgets. All durable state lives in the browser.
package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/asim/malten/internal/agent"
	"github.com/asim/malten/internal/llm"
	"github.com/asim/malten/internal/store"
)

//go:embed web/*
var webFS embed.FS

// pages holds the HTML pages, each composed from the shared base layout
// (web/base.html) plus that page's content. Parsed once at startup.
var pages = map[string]*template.Template{
	"index":  mustPage("page-index.html"),
	"issues": mustPage("page-issues.html"),
}

func mustPage(file string) *template.Template {
	return template.Must(template.ParseFS(webFS, "web/base.html", "web/"+file))
}

// pageData is the data passed to the base layout.
type pageData struct {
	Title        string
	BodyClass    string // "chat" | "doc" — selects the layout mode
	Active       string // "chat" | "issues" — highlights the nav
	ChatViewport bool   // opt into the mobile-keyboard viewport handling
}

// Server wires the agent and knowledge base to HTTP handlers.
type Server struct {
	Agent   *agent.Agent
	Store   *store.Store
	started time.Time
}

// New builds a Server.
func New(a *agent.Agent, s *store.Store) *Server {
	return &Server{Agent: a, Store: s, started: time.Now()}
}

// Handler returns the HTTP mux for the application.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/chat", s.handleChat)
	mux.HandleFunc("/api/chat/stream", s.handleChatStream)
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/issues", s.handleIssuesPage)
	// Static + PWA assets, each served from the embedded FS with an explicit
	// content type. sw.js is served no-cache so a new worker is picked up on
	// deploy; the manifest and icons make the app installable.
	mux.Handle("/app.css", staticAsset("app.css", "text/css; charset=utf-8", "public, max-age=300"))
	mux.Handle("/app.js", staticAsset("app.js", "text/javascript; charset=utf-8", "public, max-age=300"))
	mux.Handle("/manifest.webmanifest", staticAsset("manifest.webmanifest", "application/manifest+json", "public, max-age=3600"))
	mux.Handle("/sw.js", staticAsset("sw.js", "text/javascript; charset=utf-8", "no-cache"))
	mux.Handle("/icon-192.png", staticAsset("icon-192.png", "image/png", "public, max-age=86400"))
	mux.Handle("/icon-512.png", staticAsset("icon-512.png", "image/png", "public, max-age=86400"))
	mux.Handle("/icon-maskable-512.png", staticAsset("icon-maskable-512.png", "image/png", "public, max-age=86400"))
	mux.HandleFunc("/", s.handleIndex)
	return mux
}

// handleHealth is the operational check: model, uptime and knowledge-base size.
// It reports nothing about users because the server holds nothing about them.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"status":         "ok",
		"model":          s.Agent.Model.Name(),
		"uptime_seconds": int(time.Since(s.started).Seconds()),
	}
	if n, err := s.Store.KBCount(); err == nil {
		resp["kb_articles"] = n
	}
	writeJSON(w, http.StatusOK, resp)
}

// clientMessage is one prior turn as the client stores it (plain text).
type clientMessage struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

// chatRequest is the full context the client sends up for one exchange.
type chatRequest struct {
	Messages []clientMessage `json:"messages"`
	Issues   []store.Issue   `json:"issues"`
	Message  string          `json:"message"`
}

// turn converts a request into an agent.Turn.
func turn(req chatRequest) agent.Turn {
	var msgs []llm.Message
	for _, m := range req.Messages {
		if strings.TrimSpace(m.Text) == "" {
			continue
		}
		role := llm.RoleAssistant
		if m.Role == "user" {
			role = llm.RoleUser
		}
		msgs = append(msgs, llm.Message{Role: role, Content: []llm.Block{llm.Text(m.Text)}})
	}
	return agent.Turn{Messages: msgs, Issues: req.Issues, Message: strings.TrimSpace(req.Message)}
}

func decodeChat(w http.ResponseWriter, r *http.Request) (chatRequest, bool) {
	var req chatRequest
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return req, false
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return req, false
	}
	if strings.TrimSpace(req.Message) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message is required"})
		return req, false
	}
	return req, true
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeChat(w, r)
	if !ok {
		return
	}
	reply, err := s.Agent.Handle(r.Context(), turn(req))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, reply)
}

// handleChatStream is the streaming counterpart: it emits Server-Sent Events as
// the turn unfolds (assistant text deltas, tool status), then a final "done"
// event carrying the full reply (including any issue changes to save).
func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeChat(w, r)
	if !ok {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.fail(w, r, fmt.Errorf("streaming unsupported"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // let nginx flush immediately
	w.WriteHeader(http.StatusOK)

	send := func(v any) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
	}

	reply, err := s.Agent.HandleStream(r.Context(), turn(req), func(ev agent.StreamEvent) {
		if ev.Delta != "" {
			send(map[string]any{"type": "delta", "text": ev.Delta})
		}
		if ev.Tool != "" {
			send(map[string]any{"type": "status", "tool": ev.Tool})
		}
	})
	if err != nil {
		log.Printf("malten: stream: %v", err)
		send(map[string]any{"type": "error", "error": "Sorry, something went wrong. Please try again."})
		return
	}
	send(map[string]any{"type": "done", "reply": reply})
}

// fail logs the real error server-side and returns a generic message to the
// client so internal details are never shown to users.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	log.Printf("malten: %s %s: %v", r.Method, r.URL.Path, err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{
		"error": "Sorry, something went wrong on our end. Please try again.",
	})
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	renderPage(w, "index", pageData{Title: "Malten", BodyClass: "chat", Active: "chat", ChatViewport: true})
}

func (s *Server) handleIssuesPage(w http.ResponseWriter, r *http.Request) {
	renderPage(w, "issues", pageData{Title: "Malten — Issues", BodyClass: "doc", Active: "issues"})
}

// renderPage executes a page through the shared base layout.
func renderPage(w http.ResponseWriter, name string, data pageData) {
	t, ok := pages[name]
	if !ok {
		http.Error(w, "ui not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "base.html", data); err != nil {
		log.Printf("malten: render %s: %v", name, err)
	}
}

// staticAsset serves a single embedded file from web/ with an explicit content
// type and cache policy.
func staticAsset(file, contentType, cacheControl string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := webFS.ReadFile("web/" + file)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", contentType)
		if cacheControl != "" {
			w.Header().Set("Cache-Control", cacheControl)
		}
		_, _ = w.Write(data)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
