package server

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
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

// logTileFailure reports each (zoom, status) pair once — a failing zoom level
// produces a tile error per tile, and the operator only needs to hear it once.
var tileFailSeen sync.Map

func logTileFailure(z, status int) {
	key := z<<16 | status
	if _, dup := tileFailSeen.LoadOrStore(key, true); dup {
		return
	}
	msg := fmt.Sprintf("os maps: zoom %d returned %d", z, status)
	if status == http.StatusForbidden {
		msg += " — the OS Data Hub project for OS_API_KEY doesn't cover this zoom level (add the Premium plan for the detailed levels); the map falls back to scaling up the deepest level it can fetch"
	}
	log.Print(msg)
}

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
		// without the body, so Leaflet's tileerror fires cleanly. Log it — a 403
		// on the detailed zooms means the key's OS Data Hub project is on the
		// OpenData plan, which doesn't reach them, and there's no other way for
		// the operator to find that out.
		logTileFailure(z, resp.StatusCode)
		http.Error(w, "tile unavailable", resp.StatusCode)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = io.Copy(w, resp.Body)
}
