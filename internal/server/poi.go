package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/asim/malten/internal/llm"
)

// poi.go proxies the OpenStreetMap Overpass API for named points of interest
// near a point — pubs, parks, landmarks, viewpoints, historic sites. It's what
// the camera "look around" mode tags in view. Overpass is free and keyless; the
// response is JSON, so this stays dependency-free (encoding/json). OSM data is
// ODbL — the UI attributes it.
//
// Results are cached server-side by grid cell: the first request in a cell
// fetches a padded bounding box once, and every later request whose point falls
// in that cell is served from memory — so wandering around an area never re-hits
// Overpass. The cache holds only public place data (no user content), in memory,
// with a short TTL; it never touches disk.

var overpassClient = &http.Client{Timeout: 18 * time.Second}

// overpassEndpoint is the Overpass instance to query. Defaults to the main
// public server; override with MALTEN_OVERPASS_URL to use a mirror (also handy
// for tests).
var overpassEndpoint = envOr("MALTEN_OVERPASS_URL", "https://overpass-api.de/api/interpreter")

// POI is a named point of interest. The contact tags are what let the agent
// answer "is it open?" — they're whatever OSM contributors have recorded, so
// they can be missing or out of date.
type POI struct {
	Name  string  `json:"name"`
	Kind  string  `json:"kind"`
	Lat   float64 `json:"lat"`
	Lng   float64 `json:"lng"`
	Hours string  `json:"hours,omitempty"` // OSM opening_hours syntax, e.g. "Mo-Su 09:00-17:00"
	Web   string  `json:"web,omitempty"`
	Phone string  `json:"phone,omitempty"`
}

// --- bounding-box cache -----------------------------------------------------

const (
	poiCellDeg = 0.02             // ~2.2 km grid cell
	poiPadDeg  = 0.009            // ~1 km padding, so a point anywhere in the cell
	poiTTL     = 10 * time.Minute // has full coverage around it
)

type poiCell struct {
	mu   sync.Mutex
	pois []POI
	at   time.Time
}

var (
	poiCellsMu sync.Mutex
	poiCells   = map[string]*poiCell{}
)

// cellFor returns the cache key and the padded bounding box (S, W, N, E) for the
// grid cell containing a point.
func cellFor(lat, lng float64) (key string, south, west, north, east float64) {
	ci, cj := math.Floor(lat/poiCellDeg), math.Floor(lng/poiCellDeg)
	south = ci*poiCellDeg - poiPadDeg
	north = (ci+1)*poiCellDeg + poiPadDeg
	west = cj*poiCellDeg - poiPadDeg
	east = (cj+1)*poiCellDeg + poiPadDeg
	return fmt.Sprintf("%d:%d", int(ci), int(cj)), south, west, north, east
}

// cellPOIs returns the POIs for the cell containing the point, fetching from
// Overpass only on a cache miss or when the entry has gone stale. The per-cell
// lock serializes concurrent misses so a busy cell hits Overpass once.
func cellPOIs(lat, lng float64) ([]POI, error) {
	key, s, w, n, e := cellFor(lat, lng)

	poiCellsMu.Lock()
	c := poiCells[key]
	if c == nil {
		c = &poiCell{}
		poiCells[key] = c
	}
	poiCellsMu.Unlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.at.IsZero() && time.Since(c.at) < poiTTL {
		return c.pois, nil
	}
	pois, err := runOverpass(bboxQuery(s, w, n, e))
	if err != nil {
		if !c.at.IsZero() {
			return c.pois, nil // serve stale rather than fail
		}
		return nil, err
	}
	c.pois, c.at = pois, time.Now()
	return pois, nil
}

// --- Overpass ---------------------------------------------------------------

// categories is the shared set of feature filters (applied to nodes and, where
// it makes sense, ways). %s is the area clause (around:… or a bbox).
func overpassBody(area string) string {
	n := func(f string) string { return "node(" + area + ")[name]" + f + ";" }
	way := func(f string) string { return "way(" + area + ")[name]" + f + ";" }
	return "[out:json][timeout:25];(" +
		n("[tourism]") + way("[tourism]") +
		n("[historic]") + way("[historic]") +
		n(`[leisure~"^(park|garden|nature_reserve|common|pitch|stadium|sports_centre)$"]`) +
		way(`[leisure~"^(park|garden|nature_reserve|common|stadium|sports_centre)$"]`) +
		n(`[natural~"^(peak|water|beach|wood|spring|cliff)$"]`) +
		n(`[amenity~"^(pub|cafe|bar|restaurant|theatre|cinema|marketplace|place_of_worship|library|arts_centre)$"]`) +
		");out center 300;"
}

func bboxQuery(s, w, n, e float64) string {
	return overpassBody(fmt.Sprintf("%.6f,%.6f,%.6f,%.6f", s, w, n, e))
}

// fetchPOIs runs an around-radius query (used directly by tests and callers that
// want a specific radius rather than the cell cache).
func fetchPOIs(lat, lng float64, radius int) ([]POI, error) {
	if radius <= 0 || radius > 2000 {
		radius = 400
	}
	return runOverpass(overpassBody(fmt.Sprintf("around:%d,%.6f,%.6f", radius, lat, lng)))
}

func runOverpass(query string) ([]POI, error) {
	form := url.Values{}
	form.Set("data", query)
	req, err := http.NewRequest(http.MethodPost, overpassEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Malten/1.0 (+https://malten.ai)")

	resp, err := overpassClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("overpass status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parsePOIs(body), nil
}

func parsePOIs(body []byte) []POI {
	var raw struct {
		Elements []struct {
			Lat    float64 `json:"lat"`
			Lon    float64 `json:"lon"`
			Center *struct {
				Lat float64 `json:"lat"`
				Lon float64 `json:"lon"`
			} `json:"center"`
			Tags map[string]string `json:"tags"`
		} `json:"elements"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil
	}
	seen := map[string]bool{}
	out := make([]POI, 0, len(raw.Elements))
	for _, e := range raw.Elements {
		name := e.Tags["name"]
		if name == "" {
			continue
		}
		lat, lng := e.Lat, e.Lon
		if lat == 0 && lng == 0 && e.Center != nil {
			lat, lng = e.Center.Lat, e.Center.Lon
		}
		if lat == 0 && lng == 0 {
			continue
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, POI{
			Name:  name,
			Kind:  poiKind(e.Tags),
			Lat:   lat,
			Lng:   lng,
			Hours: e.Tags["opening_hours"],
			Web:   firstTag(e.Tags, "website", "contact:website", "url"),
			Phone: firstTag(e.Tags, "phone", "contact:phone"),
		})
	}
	return out
}

// firstTag returns the first of keys that is present and non-empty.
func firstTag(tags map[string]string, keys ...string) string {
	for _, k := range keys {
		if v := tags[k]; v != "" {
			return v
		}
	}
	return ""
}

// poiKind picks the most descriptive tag value for a feature.
func poiKind(tags map[string]string) string {
	for _, k := range []string{"tourism", "historic", "leisure", "natural", "amenity", "shop"} {
		if v := tags[k]; v != "" {
			return strings.ReplaceAll(v, "_", " ")
		}
	}
	return "place"
}

// poiTools exposes OpenStreetMap's named places to the agent. It's what lets a
// question like "is the café by the gate open?" be answered from data rather
// than deflected — OSM carries opening hours, a website and a phone number for
// a lot of places, patchily.
func (s *Server) poiTools(req askRequest) []llm.Tool {
	return []llm.Tool{{
		Name: "nearby_places",
		Description: "Named places from OpenStreetMap near a point — cafés, pubs, restaurants, parks, museums, landmarks, viewpoints. " +
			"Each result has its kind, distance, and (where OSM has them) opening hours in opening_hours syntax, a website and a phone number. " +
			"Use this for questions about a specific named place nearby, or what sort of place is around. " +
			"Pass name to filter by name; omit lat/lng to use the user's current location.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":     map[string]any{"type": "string", "description": "filter to places whose name contains this (case-insensitive)"},
				"lat":      map[string]any{"type": "number", "description": "latitude; defaults to the user's location"},
				"lng":      map[string]any{"type": "number", "description": "longitude; defaults to the user's location"},
				"radius_m": map[string]any{"type": "integer", "description": "search radius in metres (default 800, max 2000)"},
			},
		},
		Run: func(ctx context.Context, in json.RawMessage) (string, error) {
			var a struct {
				Name   string   `json:"name"`
				Lat    *float64 `json:"lat"`
				Lng    *float64 `json:"lng"`
				Radius int      `json:"radius_m"`
			}
			_ = json.Unmarshal(in, &a)
			lat, lng := req.Lat, req.Lng
			if a.Lat != nil {
				lat = *a.Lat
			}
			if a.Lng != nil {
				lng = *a.Lng
			}
			if lat == 0 && lng == 0 {
				return "No location available. Ask the user where they are, or use find_place.", nil
			}
			if a.Radius <= 0 {
				a.Radius = 800
			}
			pois, err := fetchPOIs(lat, lng, a.Radius)
			if err != nil {
				return "The places service is unavailable right now.", nil
			}
			if q := strings.ToLower(strings.TrimSpace(a.Name)); q != "" {
				var hit []POI
				for _, p := range pois {
					if strings.Contains(strings.ToLower(p.Name), q) {
						hit = append(hit, p)
					}
				}
				if len(hit) == 0 {
					return fmt.Sprintf("Nothing named like %q is mapped within %dm. It may be there but unmapped, or further away — try a wider radius.", a.Name, a.Radius), nil
				}
				pois = hit
			}
			sort.Slice(pois, func(i, j int) bool {
				return sq(pois[i].Lat-lat)+sq(pois[i].Lng-lng) < sq(pois[j].Lat-lat)+sq(pois[j].Lng-lng)
			})
			if len(pois) > 25 {
				pois = pois[:25]
			}
			out := make([]map[string]any, 0, len(pois))
			for _, p := range pois {
				m := map[string]any{"name": p.Name, "kind": p.Kind, "metres": int(metresBetween(lat, lng, p.Lat, p.Lng))}
				if p.Hours != "" {
					m["opening_hours"] = p.Hours
				}
				if p.Web != "" {
					m["website"] = p.Web
				}
				if p.Phone != "" {
					m["phone"] = p.Phone
				}
				out = append(out, m)
			}
			b, _ := json.Marshal(out)
			return string(b), nil
		},
	}}
}

// metresBetween is a small-distance approximation — fine at POI range.
func metresBetween(lat1, lng1, lat2, lng2 float64) float64 {
	const mPerDeg = 111320
	dLat := (lat2 - lat1) * mPerDeg
	dLng := (lng2 - lng1) * mPerDeg * math.Cos(lat1*math.Pi/180)
	return math.Hypot(dLat, dLng)
}

func (s *Server) handlePOI(w http.ResponseWriter, r *http.Request) {
	lat, err1 := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	lng, err2 := strconv.ParseFloat(r.URL.Query().Get("lng"), 64)
	if err1 != nil || err2 != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "lat and lng are required"})
		return
	}
	cached, err := cellPOIs(lat, lng)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "couldn't reach the places service"})
		return
	}
	// Copy before sorting — the cached slice is shared across requests.
	pois := make([]POI, len(cached))
	copy(pois, cached)
	sort.Slice(pois, func(i, j int) bool {
		return sq(pois[i].Lat-lat)+sq(pois[i].Lng-lng) < sq(pois[j].Lat-lat)+sq(pois[j].Lng-lng)
	})
	if len(pois) > 120 {
		pois = pois[:120]
	}
	writeJSON(w, http.StatusOK, map[string]any{"pois": pois})
}

func sq(x float64) float64 { return x * x }
