package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLocationAndRoute(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/reverse":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"Richmond Park","address":{"city":"London","country":"United Kingdom"}}`))
		case "/route/v1/driving/-0.300000,51.450000;-0.200000,51.500000":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"routes":[{"distance":9200,"duration":1140}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	oldNominatim, oldOSRM := nominatimEndpoint, osrmEndpoint
	nominatimEndpoint, osrmEndpoint = upstream.URL, upstream.URL
	locationCache.Lock()
	locationCache.places = make(map[string]locationPlace)
	locationCache.Unlock()
	t.Cleanup(func() { nominatimEndpoint, osrmEndpoint = oldNominatim, oldOSRM })

	s := New()
	for _, tc := range []struct{ path, want string }{
		{"/api/location?lat=51.45&lng=-0.3", `"name":"Richmond Park"`},
		{"/api/route?from_lat=51.45&from_lng=-0.3&to_lat=51.5&to_lng=-0.2", `"distance_metres":9200`},
	} {
		recorder := httptest.NewRecorder()
		s.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if recorder.Code != http.StatusOK { t.Fatalf("%s: status %d: %s", tc.path, recorder.Code, recorder.Body.String()) }
		if body := recorder.Body.String(); !contains(body, tc.want) { t.Fatalf("%s: %s does not contain %s", tc.path, body, tc.want) }
	}
}

func contains(s, part string) bool {
	for i := 0; i+len(part) <= len(s); i++ { if s[i:i+len(part)] == part { return true } }
	return false
}
