package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

// The contact tags are what let the agent answer "is it open?", so they have to
// survive parsing — including the contact:-prefixed spellings.
func TestParsePOIContactTags(t *testing.T) {
	body := `{"elements":[
		{"type":"node","id":1,"lat":51.4045,"lon":-0.3372,"tags":{"name":"Lion Gate Café","amenity":"cafe","opening_hours":"Mo-Su 09:00-17:00","contact:website":"https://example.com","contact:phone":"+44 20 7946 0000"}},
		{"type":"node","id":2,"lat":51.4046,"lon":-0.3371,"tags":{"name":"Tiltyard","amenity":"cafe","website":"https://tiltyard.example","phone":"020 7946 0001"}}
	]}`
	byName := map[string]POI{}
	for _, p := range parsePOIs([]byte(body)) {
		byName[p.Name] = p
	}
	cafe := byName["Lion Gate Café"]
	if cafe.Hours != "Mo-Su 09:00-17:00" {
		t.Errorf("hours = %q", cafe.Hours)
	}
	if cafe.Web != "https://example.com" || cafe.Phone != "+44 20 7946 0000" {
		t.Errorf("contact: tags not picked up: %+v", cafe)
	}
	if got := byName["Tiltyard"]; got.Web != "https://tiltyard.example" || got.Phone != "020 7946 0001" {
		t.Errorf("plain tags not picked up: %+v", got)
	}
	// Absent tags stay empty rather than turning up as "".
	if b, _ := json.Marshal(byName["Tiltyard"]); strings.Contains(string(b), `"hours"`) {
		t.Errorf("empty hours serialised: %s", b)
	}
}

// nearby_places is the tool that answers questions about a named place.
func TestNearbyPlacesTool(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"elements":[
			{"type":"node","id":1,"lat":51.4045,"lon":-0.3372,"tags":{"name":"Lion Gate Café","amenity":"cafe","opening_hours":"Mo-Su 09:00-17:00"}},
			{"type":"node","id":2,"lat":51.5000,"lon":-0.3000,"tags":{"name":"Far Away Inn","amenity":"pub"}}
		]}`))
	}))
	defer srv.Close()
	old := overpassEndpoint
	overpassEndpoint = srv.URL
	defer func() { overpassEndpoint = old }()

	s := &Server{}
	tool := s.poiTools(askRequest{Lat: 51.4036, Lng: -0.3378, HasLoc: true})[0]
	if tool.Name != "nearby_places" {
		t.Fatalf("tool name = %q", tool.Name)
	}

	out, err := tool.Run(context.Background(), json.RawMessage(`{"name":"lion gate"}`))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "Lion Gate Café") || !strings.Contains(out, "Mo-Su 09:00-17:00") {
		t.Errorf("hours not passed to the model: %s", out)
	}
	if strings.Contains(out, "Far Away Inn") {
		t.Errorf("name filter leaked other places: %s", out)
	}
	if !strings.Contains(out, `"metres"`) {
		t.Errorf("no distance in the result: %s", out)
	}

	// A miss must say the place isn't mapped, not that it can't be checked.
	out, _ = tool.Run(context.Background(), json.RawMessage(`{"name":"nonesuch tavern"}`))
	if !strings.Contains(out, "Nothing named") {
		t.Errorf("miss = %q", out)
	}
}

func TestCellCaching(t *testing.T) {
	var hits int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		_, _ = w.Write([]byte(sampleOverpass))
	}))
	defer srv.Close()

	old := overpassEndpoint
	overpassEndpoint = srv.URL
	defer func() { overpassEndpoint = old }()
	// Isolate the cache for the test.
	poiCellsMu.Lock()
	poiCells = map[string]*poiCell{}
	poiCellsMu.Unlock()

	// Two nearby points inside the same ~2km cell → one upstream fetch.
	if _, err := cellPOIs(51.5080, -0.1281); err != nil {
		t.Fatal(err)
	}
	if _, err := cellPOIs(51.5090, -0.1290); err != nil {
		t.Fatal(err)
	}
	// A point in a far-away cell → a second fetch.
	if _, err := cellPOIs(53.4800, -2.2400); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if hits != 2 {
		t.Errorf("overpass hit %d times, want 2 (same cell cached, new cell fetched)", hits)
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
