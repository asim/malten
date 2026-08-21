package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

const sampleOverpass = `{
  "elements": [
    {"type":"node","id":1,"lat":51.5081,"lon":-0.1283,"tags":{"name":"National Gallery","tourism":"museum"}},
    {"type":"node","id":2,"lat":51.5079,"lon":-0.1280,"tags":{"name":"The Chandos","amenity":"pub"}},
    {"type":"way","id":3,"center":{"lat":51.5074,"lon":-0.1278},"tags":{"name":"St Martin-in-the-Fields","historic":"church"}},
    {"type":"node","id":4,"lat":51.5085,"lon":-0.1290,"tags":{"amenity":"bench"}},
    {"type":"node","id":5,"lat":51.5079,"lon":-0.1280,"tags":{"name":"The Chandos","amenity":"pub"}},
    {"type":"node","id":6,"lat":51.5090,"lon":-0.1300,"tags":{"name":"Trafalgar Square","natural":"","leisure":"","tourism":"attraction"}}
  ]
}`

func TestParsePOIs(t *testing.T) {
	pois := parsePOIs([]byte(sampleOverpass))
	// name-less (id 4) skipped; duplicate "The Chandos" (id 5) collapsed.
	if len(pois) != 4 {
		t.Fatalf("got %d POIs, want 4: %+v", len(pois), pois)
	}
	byName := map[string]POI{}
	for _, p := range pois {
		byName[p.Name] = p
	}
	if byName["National Gallery"].Kind != "museum" {
		t.Errorf("National Gallery kind = %q, want museum", byName["National Gallery"].Kind)
	}
	if byName["The Chandos"].Kind != "pub" {
		t.Errorf("The Chandos kind = %q, want pub", byName["The Chandos"].Kind)
	}
	// way center used for coordinates.
	if got := byName["St Martin-in-the-Fields"]; got.Lat != 51.5074 || got.Lng != -0.1278 {
		t.Errorf("church center not used: %+v", got)
	}
	// tourism preferred over empty tags.
	if byName["Trafalgar Square"].Kind != "attraction" {
		t.Errorf("Trafalgar Square kind = %q, want attraction", byName["Trafalgar Square"].Kind)
	}
}

func TestFetchPOIsHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST")
		}
		if err := r.ParseForm(); err != nil || r.Form.Get("data") == "" {
			t.Errorf("missing overpass query")
		}
		_, _ = w.Write([]byte(sampleOverpass))
	}))
	defer srv.Close()

	old := overpassEndpoint
	overpassEndpoint = srv.URL
	defer func() { overpassEndpoint = old }()

	pois, err := fetchPOIs(51.508, -0.1281, 300)
	if err != nil {
		t.Fatalf("fetchPOIs: %v", err)
	}
	if len(pois) != 4 {
		t.Fatalf("got %d POIs, want 4", len(pois))
	}
}
