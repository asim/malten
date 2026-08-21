package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// poi.go proxies the OpenStreetMap Overpass API for named points of interest
// near a point — pubs, parks, landmarks, viewpoints, historic sites. It's what
// the camera "look around" mode tags in view. Overpass is free and keyless;
// the response is JSON, so this stays dependency-free (encoding/json). OSM data
// is ODbL — the UI attributes it.

var overpassClient = &http.Client{Timeout: 20 * time.Second}

// overpassEndpoint is the Overpass instance to query. Defaults to the main
// public server; override with MALTEN_OVERPASS_URL to use a mirror (also handy
// for tests).
var overpassEndpoint = envOr("MALTEN_OVERPASS_URL", "https://overpass-api.de/api/interpreter")

// POI is a named point of interest.
type POI struct {
	Name string  `json:"name"`
	Kind string  `json:"kind"`
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
}

// overpassQuery builds an Overpass QL query for interesting, named features
// within radius metres of a point.
func overpassQuery(lat, lng float64, radius int) string {
	a := fmt.Sprintf("around:%d,%.6f,%.6f", radius, lat, lng)
	return "[out:json][timeout:20];(" +
		"node(" + a + ")[name][tourism];" +
		"way(" + a + ")[name][tourism];" +
		"node(" + a + ")[name][historic];" +
		"way(" + a + ")[name][historic];" +
		`node(` + a + `)[name][leisure~"^(park|garden|nature_reserve|common|pitch|stadium|sports_centre)$"];` +
		`way(` + a + `)[name][leisure~"^(park|garden|nature_reserve|common|stadium|sports_centre)$"];` +
		`node(` + a + `)[name][natural~"^(peak|water|beach|wood|spring|cliff)$"];` +
		`node(` + a + `)[name][amenity~"^(pub|cafe|bar|restaurant|theatre|cinema|marketplace|place_of_worship|library|arts_centre)$"];` +
		");out center 80;"
}

// fetchPOIs runs an Overpass query and returns nearby POIs.
func fetchPOIs(lat, lng float64, radius int) ([]POI, error) {
	if radius <= 0 || radius > 2000 {
		radius = 400
	}
	form := url.Values{}
	form.Set("data", overpassQuery(lat, lng, radius))
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
		if seen[name] { // collapse duplicate names (a place mapped as several features)
			continue
		}
		seen[name] = true
		out = append(out, POI{Name: name, Kind: poiKind(e.Tags), Lat: lat, Lng: lng})
	}
	return out
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

func (s *Server) handlePOI(w http.ResponseWriter, r *http.Request) {
	lat, err1 := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	lng, err2 := strconv.ParseFloat(r.URL.Query().Get("lng"), 64)
	if err1 != nil || err2 != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "lat and lng are required"})
		return
	}
	radius, _ := strconv.Atoi(r.URL.Query().Get("radius"))
	pois, err := fetchPOIs(lat, lng, radius)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "couldn't reach the places service"})
		return
	}
	// Nearest first (by rough squared distance; fine at these scales).
	sort.Slice(pois, func(i, j int) bool {
		di := sq(pois[i].Lat-lat) + sq(pois[i].Lng-lng)
		dj := sq(pois[j].Lat-lat) + sq(pois[j].Lng-lng)
		return di < dj
	})
	writeJSON(w, http.StatusOK, map[string]any{"pois": pois})
}

func sq(x float64) float64 { return x * x }
