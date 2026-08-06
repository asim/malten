// Package server exposes the agent over HTTP and serves the embedded chat UI.
//
// Session handling is deliberately lightweight (no accounts/login): the client
// may supply a session_id, or the server mints one on first contact and returns
// it. History is keyed by that id in the store. A cookie is also set as a
// convenience so a browser reload keeps its session.
package server

import (
	"embed"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/asim/malten/internal/agent"
	"github.com/asim/malten/internal/id"
	"github.com/asim/malten/internal/store"
)

//go:embed web/*
var webFS embed.FS

// pages holds the HTML pages, each composed from the shared base layout
// (web/base.html) plus that page's content — so every page shares one header,
// theme and page width. Parsed once at startup from the embedded FS.
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

// Server wires the agent and store to HTTP handlers.
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
	mux.HandleFunc("/api/session/", s.handleSession)
	mux.HandleFunc("/api/issues", s.handleIssues)
	mux.HandleFunc("/issues", s.handleIssuesPage)
	mux.HandleFunc("/api/health", s.handleHealth)
	// Static + PWA assets, each served from the embedded FS with an explicit
	// content type. sw.js is served no-cache so a new worker is picked up on
	// deploy; the manifest and icons make the app installable.
	mux.Handle("/app.css", staticAsset("app.css", "text/css; charset=utf-8", "public, max-age=300"))
	mux.Handle("/manifest.webmanifest", staticAsset("manifest.webmanifest", "application/manifest+json", "public, max-age=3600"))
	mux.Handle("/sw.js", staticAsset("sw.js", "text/javascript; charset=utf-8", "no-cache"))
	mux.Handle("/icon-192.png", staticAsset("icon-192.png", "image/png", "public, max-age=86400"))
	mux.Handle("/icon-512.png", staticAsset("icon-512.png", "image/png", "public, max-age=86400"))
	mux.Handle("/icon-maskable-512.png", staticAsset("icon-maskable-512.png", "image/png", "public, max-age=86400"))
	mux.HandleFunc("/", s.handleIndex)
	return mux
}

// handleHealth is the operational check: model, uptime and row counts. Handy
// for spotting whether the persisted store looks as expected after a restart.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"status":         "ok",
		"model":          s.Agent.Model.Name(),
		"uptime_seconds": int(time.Since(s.started).Seconds()),
	}
	if stats, err := s.Store.Stats(); err == nil {
		resp["sessions"] = stats.Sessions
		resp["messages"] = stats.Messages
		resp["issues"] = stats.Issues
	}
	writeJSON(w, http.StatusOK, resp)
}

type chatRequest struct {
	SessionID string `json:"session_id"`
	Message   string `json:"message"`
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message is required"})
		return
	}

	// Resolve session: body, then cookie, then mint a new one.
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		if c, err := r.Cookie("malten_session"); err == nil {
			sessionID = c.Value
		}
	}
	if sessionID == "" {
		if err := s.ensureSession(id.New("SESS"), &sessionID); err != nil {
			s.fail(w, r, err)
			return
		}
	} else if ok, _ := s.Store.SessionExists(sessionID); !ok {
		if err := s.Store.CreateSession(sessionID); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	http.SetCookie(w, &http.Cookie{Name: "malten_session", Value: sessionID, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})

	reply, err := s.Agent.Handle(r.Context(), sessionID, req.Message)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, reply)
}

// ensureSession creates a session, retrying with a fresh id on the astronomically
// unlikely event of an id collision, and writes the id actually used to out.
func (s *Server) ensureSession(candidate string, out *string) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if err := s.Store.CreateSession(candidate); err == nil {
			*out = candidate
			return nil
		} else {
			lastErr = err
		}
		candidate = id.New("SESS")
	}
	return lastErr
}

// fail logs the real error server-side and returns a generic message to the
// client so internal details (e.g. SQL errors) are never shown to users.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	log.Printf("malten: %s %s: %v", r.Method, r.URL.Path, err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{
		"error": "Sorry, something went wrong on our end. Please try again.",
	})
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimPrefix(r.URL.Path, "/api/session/")
	if sessionID == "" {
		http.Error(w, "session id required", http.StatusBadRequest)
		return
	}
	msgs, err := s.Store.LoadMessages(sessionID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"session_id": sessionID, "messages": msgs})
}

func (s *Server) handleIssues(w http.ResponseWriter, r *http.Request) {
	issues, err := s.Store.ListIssues()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"issues": issues})
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
// type and cache policy. Used for the stylesheet and the PWA assets (manifest,
// service worker, icons).
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
