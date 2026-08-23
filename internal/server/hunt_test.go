package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The model's reply is JSON, but models are not always tidy about it.
func TestParseHunt(t *testing.T) {
	items := parseHunt("```json\n{\"items\":[{\"what\":\"A red postbox\",\"hint\":\"Look for the crown\"}," +
		"{\"what\":\"Moss growing on a wall\"},{\"what\":\"\",\"hint\":\"dropped: nothing to find\"}]}\n```")
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2: %+v", len(items), items)
	}
	if items[0].What != "A red postbox" || items[0].Hint != "Look for the crown" {
		t.Errorf("first item = %+v", items[0])
	}
	if items[1].Hint != "" {
		t.Errorf("hint invented: %+v", items[1])
	}

	// Five is the whole hunt; more than that is the model running on.
	var many strings.Builder
	many.WriteString(`{"items":[`)
	for i := 0; i < 9; i++ {
		if i > 0 {
			many.WriteString(",")
		}
		many.WriteString(`{"what":"thing"}`)
	}
	many.WriteString(`]}`)
	if n := len(parseHunt(many.String())); n != 5 {
		t.Errorf("got %d items, want 5", n)
	}

	// Nothing usable is an empty hunt, not a crash or a blank line.
	for _, junk := range []string{"", "sorry, I can't do that", "{", `{"items":"nope"}`} {
		if n := len(parseHunt(junk)); n != 0 {
			t.Errorf("parseHunt(%q) = %d items", junk, n)
		}
	}
}

// The hunt must be built from what's really nearby, and told who it's for —
// that's the whole difference between this and a generic list.
func TestHuntPrompt(t *testing.T) {
	var prompt string
	s := testServer(t, `{"items":[{"what":"A red postbox","hint":"Look for the crown"}]}`, &prompt)

	// After testServer, so this stub is the one the hunt actually queries.
	overpass := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"elements":[
			{"type":"node","id":1,"lat":51.4045,"lon":-0.3372,"tags":{"name":"Lion Gate Café","amenity":"cafe"}},
			{"type":"node","id":2,"lat":51.4040,"lon":-0.3380,"tags":{"name":"Hampton Court Palace","historic":"castle"}}
		]}`))
	}))
	defer overpass.Close()
	old := overpassEndpoint
	overpassEndpoint = overpass.URL
	defer func() { overpassEndpoint = old }()

	rec := httptest.NewRecorder()
	s.handleHunt(rec, httptest.NewRequest(http.MethodPost, "/api/hunt",
		strings.NewReader(`{"lat":51.4036,"lng":-0.3378,"age":6}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "A red postbox") {
		t.Errorf("body = %s", rec.Body.String())
	}
	for _, want := range []string{
		"Lion Gate Café",       // the real places it's allowed to name…
		"Hampton Court Palace", //
		"NEVER invent",         // …and the instruction not to make any up
		"The child is 6 years old",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("hunt prompt missing %q:\n%s", want, prompt)
		}
	}
}

// An age that can't be right shouldn't produce a hunt for a 900-year-old.
func TestHuntAgeFallback(t *testing.T) {
	var prompt string
	s := testServer(t, `{"items":[{"what":"A bench"}]}`, &prompt)
	s.handleHunt(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/hunt",
		strings.NewReader(`{"lat":51.4036,"lng":-0.3378,"age":900}`)))
	if !strings.Contains(prompt, "The child is 6 years old") {
		t.Errorf("age not defaulted:\n%s", prompt)
	}
}

// Outside Great Britain there's no snapshot to build a hunt from.
func TestHuntOutsideGB(t *testing.T) {
	s := testServer(t, `{"items":[{"what":"nope"}]}`, nil)
	rec := httptest.NewRecorder()
	s.handleHunt(rec, httptest.NewRequest(http.MethodPost, "/api/hunt",
		strings.NewReader(`{"lat":48.8566,"lng":2.3522,"age":6}`))) // Paris
	if !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Errorf("body = %s", rec.Body.String())
	}
}
