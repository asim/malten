package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	nominatimEndpoint = envOr("MALTEN_NOMINATIM_URL", "https://nominatim.openstreetmap.org")
	locationClient    = &http.Client{Timeout: 12 * time.Second}
	locationCache     = struct {
		sync.Mutex
		places map[string]locationPlace
	}{places: make(map[string]locationPlace)}
)

type locationPlace struct {
	Name    string `json:"name"`
	Locality string `json:"locality,omitempty"`
	Country string `json:"country,omitempty"`
}


func coordinates(r *http.Request, names ...string) ([]float64, error) {
	values := make([]float64, len(names))
	for i, name := range names {
		value, err := strconv.ParseFloat(r.URL.Query().Get(name), 64)
		if err != nil {
			return nil, fmt.Errorf("%s is required", name)
		}
		values[i] = value
	}
	return values, nil
}

func (s *Server) handleLocation(w http.ResponseWriter, r *http.Request) {
	coords, err := coordinates(r, "lat", "lng")
	if err != nil || coords[0] < -90 || coords[0] > 90 || coords[1] < -180 || coords[1] > 180 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "valid lat and lng are required"})
		return
	}

	key := fmt.Sprintf("%.4f,%.4f", coords[0], coords[1])
	locationCache.Lock()
	place, ok := locationCache.places[key]
	locationCache.Unlock()
	if !ok {
		place, err = reverseLocation(coords[0], coords[1])
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "place context is unavailable"})
			return
		}
		locationCache.Lock()
		locationCache.places[key] = place
		locationCache.Unlock()
	}
	writeJSON(w, http.StatusOK, place)
}

func reverseLocation(lat, lng float64) (locationPlace, error) {
	q := url.Values{"format": {"jsonv2"}, "lat": {strconv.FormatFloat(lat, 'f', 6, 64)}, "lon": {strconv.FormatFloat(lng, 'f', 6, 64)}, "zoom": {"16"}, "addressdetails": {"1"}}
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(nominatimEndpoint, "/")+"/reverse?"+q.Encode(), nil)
	if err != nil { return locationPlace{}, err }
	req.Header.Set("User-Agent", "Malten/1.0 (+https://malten.ai)")
	resp, err := locationClient.Do(req)
	if err != nil { return locationPlace{}, err }
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK { return locationPlace{}, fmt.Errorf("nominatim status %d", resp.StatusCode) }
	body, err := io.ReadAll(resp.Body)
	if err != nil { return locationPlace{}, err }
	var raw struct {
		DisplayName string `json:"display_name"`
		Name string `json:"name"`
		Address map[string]string `json:"address"`
	}
	if err := json.Unmarshal(body, &raw); err != nil { return locationPlace{}, err }
	name := firstNonEmpty(raw.Name, raw.Address["road"], raw.Address["neighbourhood"], raw.DisplayName)
	locality := firstNonEmpty(raw.Address["city"], raw.Address["town"], raw.Address["village"], raw.Address["county"])
	return locationPlace{Name: name, Locality: locality, Country: raw.Address["country"]}, nil
}

func firstNonEmpty(values ...string) string {
 for _, value := range values { if value != "" { return value } }
 return ""
}
