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
	"net/http"
	"strings"

	"github.com/asim/malten/internal/agent"
	"github.com/asim/malten/internal/id"
	"github.com/asim/malten/internal/store"
)

//go:embed web/*
var webFS embed.FS

// Server wires the agent and store to HTTP handlers.
type Server struct {
	Agent *agent.Agent
	Store *store.Store
}

// New builds a Server.
func New(a *agent.Agent, s *store.Store) *Server {
	return &Server{Agent: a, Store: s}
}

// Handler returns the HTTP mux for the application.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/chat", s.handleChat)
	mux.HandleFunc("/api/session/", s.handleSession)
	mux.HandleFunc("/api/tickets", s.handleTickets)
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "model": s.Agent.Model.Name()})
	})
	mux.HandleFunc("/", s.handleIndex)
	return mux
}

type chatRequest struct {
	SessionID  string `json:"session_id"`
	CustomerID string `json:"customer_id"`
	Message    string `json:"message"`
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
		sessionID = id.New("SESS")
		if err := s.Store.CreateSession(sessionID, req.CustomerID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	} else if ok, _ := s.Store.SessionExists(sessionID); !ok {
		if err := s.Store.CreateSession(sessionID, req.CustomerID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}
	http.SetCookie(w, &http.Cookie{Name: "malten_session", Value: sessionID, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})

	reply, err := s.Agent.Handle(r.Context(), sessionID, strings.TrimSpace(req.CustomerID), req.Message)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, reply)
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimPrefix(r.URL.Path, "/api/session/")
	if sessionID == "" {
		http.Error(w, "session id required", http.StatusBadRequest)
		return
	}
	msgs, err := s.Store.LoadMessages(sessionID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"session_id": sessionID, "messages": msgs})
}

func (s *Server) handleTickets(w http.ResponseWriter, r *http.Request) {
	tickets, err := s.Store.ListTickets()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tickets": tickets})
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := webFS.ReadFile("web/index.html")
	if err != nil {
		http.Error(w, "ui not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
