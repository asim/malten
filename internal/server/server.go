// Package server serves the Malten spatial-exploration UI and a couple of tiny
// stateless helpers. It stores nothing: your finds and your OS API key live in
// the browser. Map tiles are fetched by the client straight from the Ordnance
// Survey APIs with your key; the server never sees it.
package server

import (
	"embed"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/asim/malten/internal/llm"
	"github.com/asim/malten/internal/osgrid"
)

//go:embed web/*
var webFS embed.FS

var page = template.Must(template.ParseFS(webFS, "web/base.html", "web/page-map.html"))

// Server serves the app.
type Server struct {
	started    time.Time
	interest   *interestBook
	adminToken string
	llm        *llm.Client // "Ask Malten"; nil when ANTHROPIC_API_KEY is unset
}

// New builds a Server. The waitlist is stored at MALTEN_DATA (default
// interest.jsonl). The list is viewable only with the admin token, read from
// MALTEN_ADMIN_TOKEN or, failing that, an admin_token file next to the data.
//
// "Ask Malten" is enabled only when ANTHROPIC_API_KEY is present; the model can
// be overridden with MALTEN_MODEL.
func New() *Server {
	dataPath := envOr("MALTEN_DATA", "interest.jsonl")
	s := &Server{
		started:    time.Now(),
		interest:   &interestBook{path: dataPath},
		adminToken: resolveAdminToken(),
	}
	if key := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")); key != "" {
		s.llm = llm.New(key, os.Getenv("MALTEN_MODEL"))
	}
	return s
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func resolveAdminToken() string {
	if t := strings.TrimSpace(os.Getenv("MALTEN_ADMIN_TOKEN")); t != "" {
		return t
	}
	if b, err := os.ReadFile(envOr("MALTEN_ADMIN_TOKEN_FILE", "admin_token")); err == nil {
		return strings.TrimSpace(string(b))
	}
	return ""
}

// Handler returns the HTTP mux for the application.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/gridref", s.handleGridRef)
	mux.HandleFunc("/api/stops", s.handleStops)
	mux.HandleFunc("/api/arrivals", s.handleArrivals)
	mux.HandleFunc("/api/interest", s.handleInterest)
	mux.HandleFunc("/api/ask", s.handleAsk)
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.Handle("/app.css", staticAsset("app.css", "text/css; charset=utf-8", "public, max-age=300"))
	mux.Handle("/app.js", staticAsset("app.js", "text/javascript; charset=utf-8", "public, max-age=300"))
	mux.Handle("/leaflet.js", staticAsset("leaflet.js", "text/javascript; charset=utf-8", "public, max-age=86400"))
	mux.Handle("/leaflet.css", staticAsset("leaflet.css", "text/css; charset=utf-8", "public, max-age=86400"))
	mux.Handle("/manifest.webmanifest", staticAsset("manifest.webmanifest", "application/manifest+json", "public, max-age=3600"))
	mux.Handle("/sw.js", staticAsset("sw.js", "text/javascript; charset=utf-8", "no-cache"))
	mux.Handle("/icon-192.png", staticAsset("icon-192.png", "image/png", "public, max-age=86400"))
	mux.Handle("/icon-512.png", staticAsset("icon-512.png", "image/png", "public, max-age=86400"))
	mux.Handle("/icon-maskable-512.png", staticAsset("icon-maskable-512.png", "image/png", "public, max-age=86400"))
	mux.HandleFunc("/", s.handleIndex)
	return mux
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := page.ExecuteTemplate(w, "base.html", map[string]any{"Title": "Malten"}); err != nil {
		log.Printf("malten: render: %v", err)
	}
}

// handleGridRef converts a WGS84 lat/lng to an OS National Grid reference. Pure
// function, stateless: nothing is logged or stored.
func (s *Server) handleGridRef(w http.ResponseWriter, r *http.Request) {
	lat, err1 := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	lng, err2 := strconv.ParseFloat(r.URL.Query().Get("lng"), 64)
	if err1 != nil || err2 != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "lat and lng are required"})
		return
	}
	ref, ok := osgrid.FromWGS84(lat, lng)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"in_gb": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"in_gb":    true,
		"grid_ref": ref.GridRef,
		"easting":  ref.Easting,
		"northing": ref.Northing,
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":         "ok",
		"uptime_seconds": int(time.Since(s.started).Seconds()),
		"ask":            s.askEnabled(),
	})
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
