package spatial

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"
)

var (
	FoursquareAPIKey = os.Getenv("FOURSQUARE_API_KEY")
	WebSearchEnabled = true // Kill switch
)

type FoursquareResponse struct {
	Results []FoursquarePlace `json:"results"`
}

type FoursquarePlace struct {
	FsqPlaceID string  `json:"fsq_place_id"`
	Name       string  `json:"name"`
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	Distance   int     `json:"distance"`
	Website    string  `json:"website"`
	Tel        string  `json:"tel"`
	Location   struct {
		Address          string `json:"address"`
		Postcode         string `json:"postcode"`
		Locality         string `json:"locality"`
		FormattedAddress string `json:"formatted_address"`
	} `json:"location"`
	Categories []struct {
		Name string `json:"name"`
	} `json:"categories"`
}

// WebSearchPlaces searches for places using Foursquare when OSM fails
func WebSearchPlaces(query string, lat, lon float64) []*Entity {
	if !WebSearchEnabled || FoursquareAPIKey == "" {
		return nil
	}

	log.Printf("[web] Foursquare search: %s near %.4f,%.4f", query, lat, lon)

	client := &http.Client{Timeout: 10 * time.Second}

	// New Places API endpoint
	// Request fields including website and phone
	u := fmt.Sprintf("https://places-api.foursquare.com/places/search?query=%s&ll=%f,%f&radius=5000&limit=10&fields=fsq_place_id,name,latitude,longitude,location,categories,distance,website,tel",
		url.QueryEscape(query), lat, lon)

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		log.Printf("[web] Request error: %v", err)
		return nil
	}

	// Service key uses Bearer auth
	req.Header.Set("Authorization", "Bearer "+FoursquareAPIKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Places-Api-Version", "2025-06-17")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[web] Fetch error: %v", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		log.Printf("[web] Rate limited - disabling web search")
		WebSearchEnabled = false
		return nil
	}

	if resp.StatusCode != 200 {
		log.Printf("[web] HTTP %d", resp.StatusCode)
		return nil
	}

	var result FoursquareResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("[web] Parse error: %v", err)
		return nil
	}

	var entities []*Entity
	for _, place := range result.Results {
		if place.Latitude == 0 && place.Longitude == 0 {
			continue
		}

		address := place.Location.FormattedAddress
		if address == "" {
			address = place.Location.Address
			if place.Location.Postcode != "" {
				if address != "" {
					address += ", "
				}
				address += place.Location.Postcode
			}
		}

		var category string
		if len(place.Categories) > 0 {
			category = place.Categories[0].Name
		}

		data := map[string]interface{}{
			"address":  address,
			"category": category,
			"source":   "foursquare",
			"distance": float64(place.Distance),
		}
		if place.Website != "" {
			data["website"] = place.Website
		}
		if place.Tel != "" {
			data["phone"] = place.Tel
		}

		entity := &Entity{
			ID:   GenerateID(EntityPlace, place.Latitude, place.Longitude, place.Name),
			Type: EntityPlace,
			Name: place.Name,
			Lat:  place.Latitude,
			Lon:  place.Longitude,
			Data: data,
		}
		entities = append(entities, entity)
	}

	log.Printf("[web] Found %d places", len(entities))
	return entities
}

// WebSearch does a general web search - not supported with Foursquare
func WebSearch(query string) string {
	return ""
}

// DisableWebSearch turns off web search (kill switch)
func DisableWebSearch() {
	WebSearchEnabled = false
	log.Printf("[web] Disabled")
}

// EnableWebSearch turns on web search
func EnableWebSearch() {
	if FoursquareAPIKey != "" {
		WebSearchEnabled = true
		log.Printf("[web] Enabled")
	}
}

// WebSearchStatus returns current status
func WebSearchStatus() string {
	if FoursquareAPIKey == "" {
		return "no API key"
	}
	if !WebSearchEnabled {
		return "disabled"
	}
	return "enabled (Foursquare)"
}
