package server

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// tiles.go optionally proxies OS Maps API raster tiles server-side, so visitors
// don't each need to paste their own OS Data Hub key. When OS_API_KEY is set,
// the server fetches tiles from the OS with that key and streams them back; the
// key never reaches the browser. When it's unset, the app falls back to the
// per-user key entered in the UI (the original client-side model).
//
// This is the one deliberate exception to "the OS key stays the user's": here
// the operator supplies a single shared key, held server-side like the other
// secrets. It still stores no user content.

var tileClient = &http.Client{Timeout: 15 * time.Second}

func (s *Server) tilesEnabled() bool { return s.osKey != "" }

// osStyle is the OS Maps API raster style (EPSG:3857, zoom 7–20).
const osStyle = "Outdoor_3857"

// handleTiles proxies GET /api/tiles/{z}/{x}/{y}.png to the OS Maps API. z/x/y
// are validated as integers before being placed into the upstream URL.
func (s *Server) handleTiles(w http.ResponseWriter, r *http.Request) {
	if !s.tilesEnabled() {
		http.Error(w, "tiles not configured", http.StatusNotFound)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/tiles/")
	parts := strings.Split(rest, "/")
	if len(parts) != 3 {
		http.Error(w, "bad tile path", http.StatusBadRequest)
		return
	}
	z, err1 := strconv.Atoi(parts[0])
	x, err2 := strconv.Atoi(parts[1])
	y, err3 := strconv.Atoi(strings.TrimSuffix(parts[2], ".png"))
	if err1 != nil || err2 != nil || err3 != nil || z < 0 || z > 22 || x < 0 || y < 0 {
		http.Error(w, "bad tile coordinates", http.StatusBadRequest)
		return
	}

	upstream := fmt.Sprintf("https://api.os.uk/maps/raster/v1/zxy/%s/%d/%d/%d.png?key=%s",
		osStyle, z, x, y, url.QueryEscape(s.osKey))
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, upstream, nil)
	if err != nil {
		http.Error(w, "tile error", http.StatusBadGateway)
		return
	}
	resp, err := tileClient.Do(req)
	if err != nil {
		http.Error(w, "couldn't reach the map service", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// Pass the upstream status through (404 for out-of-coverage tiles, etc.)
		// without the body, so Leaflet's tileerror fires cleanly.
		http.Error(w, "tile unavailable", resp.StatusCode)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = io.Copy(w, resp.Body)
}
