package spatial

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"runtime"
)

type SupplementaryCinema struct {
	Name    string  `json:"name"`
	Lat     float64 `json:"lat"`
	Lon     float64 `json:"lon"`
	Address string  `json:"address"`
	Website string  `json:"website"`
}

type SupplementaryData struct {
	Source  string                `json:"source"`
	Updated string                `json:"updated"`
	Cinemas []SupplementaryCinema `json:"cinemas"`
}

var supplementaryCinemas []SupplementaryCinema

func init() {
	loadSupplementaryCinemas()
}

func loadSupplementaryCinemas() {
	// Find data directory relative to source file
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return
	}
	dataDir := filepath.Join(filepath.Dir(filename), "..", "data")
	cinemaFile := filepath.Join(dataDir, "cinemas.json")

	data, err := os.ReadFile(cinemaFile)
	if err != nil {
		log.Printf("[supplementary] Could not load cinemas.json: %v", err)
		return
	}

	var sd SupplementaryData
	if err := json.Unmarshal(data, &sd); err != nil {
		log.Printf("[supplementary] Could not parse cinemas.json: %v", err)
		return
	}

	supplementaryCinemas = sd.Cinemas
	log.Printf("[supplementary] Loaded %d cinemas from %s (updated %s)", len(sd.Cinemas), sd.Source, sd.Updated)
}

// GetSupplementaryCinemas returns cinemas within radius of location
func GetSupplementaryCinemas(lat, lon, radiusMeters float64) []*Entity {
	var results []*Entity
	log.Printf("[supplementary] Searching cinemas within %.0fm of %.4f,%.4f", radiusMeters, lat, lon)

	radiusKm := radiusMeters / 1000.0
	for _, c := range supplementaryCinemas {
		distKm := haversine(lat, lon, c.Lat, c.Lon)
		if distKm <= radiusKm {
			log.Printf("[supplementary] %s is %.1fkm away (within %.1fkm radius)", c.Name, distKm, radiusKm)
			data := map[string]interface{}{
				"category": "cinema",
				"address":  c.Address,
				"source":   "supplementary",
				"distance": distKm * 1000, // convert to meters
			}
			if c.Website != "" {
				data["website"] = c.Website
			}
			entity := &Entity{
				ID:   GenerateID(EntityPlace, c.Lat, c.Lon, c.Name),
				Type: EntityPlace,
				Name: c.Name,
				Lat:  c.Lat,
				Lon:  c.Lon,
				Data: data,
			}
			results = append(results, entity)
		}
	}

	return results
}

// haversine is defined in live.go
