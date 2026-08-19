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

	"github.com/asim/malten/internal/bods"
	"github.com/asim/malten/internal/llm"
	"github.com/asim/malten/internal/nrail"
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
	llm        *llm.Client   // "Ask Malten"; nil when ANTHROPIC_API_KEY is unset
	darwin     *nrail.Darwin // live rail departures; nil when NRE_LDBWS_TOKEN is unset
	bods       *bods.Client  // live buses; nil when BODS_API_KEY is unset
	osKey      string        // shared OS Maps key for the tile proxy; "" = per-user key in the browser
}

// New builds a Server. The waitlist is stored at MALTEN_DATA (default
// interest.jsonl).
//
// Every server-side secret is read the same way (see secret): an environment
// variable if set, otherwise a plain file of the same purpose sitting next to
// the binary — which is how the deploy provisions them from CI secrets without
// anyone editing the box. Each feature switches on only when its secret is
// present, and the UI hides features whose secret is absent (via /api/health).
func New() *Server {
	dataPath := envOr("MALTEN_DATA", "interest.jsonl")
	s := &Server{
		started:    time.Now(),
		interest:   &interestBook{path: dataPath},
		adminToken: secret("MALTEN_ADMIN_TOKEN", "admin_token"),
		osKey:      secret("OS_API_KEY", "os_api_key"),
	}
	if key := secret("ANTHROPIC_API_KEY", "anthropic_key"); key != "" {
		s.llm = llm.New(key, os.Getenv("MALTEN_MODEL"))
	}
	if tok := secret("NRE_LDBWS_TOKEN", "nre_ldbws_token"); tok != "" {
		s.darwin = nrail.NewDarwin(tok)
	}
	if key := secret("BODS_API_KEY", "bods_api_key"); key != "" {
		s.bods = bods.New(key)
	}
	return s
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// secret resolves a server-side secret: the environment variable envKey if set,
// otherwise the trimmed contents of file (resolved relative to the working
// directory) if it exists, otherwise "".
func secret(envKey, file string) string {
	if v := strings.TrimSpace(os.Getenv(envKey)); v != "" {
		return v
	}
	if b, err := os.ReadFile(file); err == nil {
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
	mux.HandleFunc("/api/stations", s.handleStations)
	mux.HandleFunc("/api/departures", s.handleDepartures)
	mux.HandleFunc("/api/buses", s.handleBuses)
	mux.HandleFunc("/api/tiles/", s.handleTiles)
	mux.HandleFunc("/api/search", s.handleSearch)
	mux.HandleFunc("/api/nearest", s.handleNearest)
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
		"rail":           s.railEnabled(),
		"buses":          s.busesEnabled(),
		"tiles":          s.tilesEnabled(),
		"search":         s.searchEnabled(),
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
