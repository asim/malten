// Package server serves the shared reflection stream and PWA.
package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"github.com/asim/malten/agent"
	"hash/crc32"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"
)

//go:embed web/*
var webFS embed.FS

var page = template.Must(template.ParseFS(webFS, "web/base.html", "web/page-map.html"))

// assetVer is a short content hash of the front-end assets, computed once at
// startup. It's appended to every CSS/JS URL (?v=…) so a new build busts the
// browser/service-worker cache immediately — no stale stylesheet or script can
// pair with fresh HTML. It only changes when the assets themselves change.
var assetVer = computeAssetVer()

func computeAssetVer() string {
	h := crc32.NewIEEE()
	for _, f := range []string{"web/app.css", "web/app.js", "web/base.html", "web/page-map.html"} {
		if b, err := webFS.ReadFile(f); err == nil {
			_, _ = h.Write(b)
		}
	}
	return fmt.Sprintf("%08x", h.Sum32())
}

// Server serves the app.
type Server struct {
	// ObserveStream lets an agent learn which streams are being viewed.
	ObserveStream func(string, string)
	AgentStreams  []agent.Stream
	started       time.Time
	stream        *streamStore
}

func New() *Server { return &Server{started: time.Now(), stream: newStreamStore()} }

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Handler returns the HTTP mux for the application.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/posts", s.handleStream)
	mux.HandleFunc("/api/posts/", s.handlePost)
	mux.Handle("/app.css", staticAsset("app.css", "text/css; charset=utf-8", "public, max-age=300"))
	mux.Handle("/app.js", staticAsset("app.js", "text/javascript; charset=utf-8", "public, max-age=300"))
	mux.Handle("/manifest.webmanifest", staticAsset("manifest.webmanifest", "application/manifest+json", "public, max-age=3600"))
	mux.Handle("/sw.js", staticAsset("sw.js", "text/javascript; charset=utf-8", "no-cache"))
	mux.Handle("/icon-192.png", staticAsset("icon-192.png", "image/png", "public, max-age=86400"))
	mux.Handle("/icon-512.png", staticAsset("icon-512.png", "image/png", "public, max-age=86400"))
	mux.Handle("/icon-maskable-512.png", staticAsset("icon-maskable-512.png", "image/png", "public, max-age=86400"))
	mux.Handle("/favicon.svg", staticAsset("favicon.svg", "image/svg+xml", "public, max-age=86400"))
	mux.HandleFunc("/", s.handleIndex)
	return mux
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := page.ExecuteTemplate(w, "base.html", map[string]any{"Title": "Malten", "Ver": assetVer, "AgentStreams": s.AgentStreams}); err != nil {
		log.Printf("malten: render: %v", err)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":         "ok",
		"uptime_seconds": int(time.Since(s.started).Seconds()),
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
