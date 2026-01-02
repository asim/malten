package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"malten.ai/spatial"
)

// PlaceHandler returns details for a specific place by ID
func PlaceHandler(w http.ResponseWriter, r *http.Request) {
	// Extract ID from /place/{id}
	id := strings.TrimPrefix(r.URL.Path, "/place/")
	if id == "" {
		http.Error(w, "Missing place ID", 400)
		return
	}

	db := spatial.Get()
	entity := db.GetByID(id)
	if entity == nil {
		http.Error(w, "Place not found", 404)
		return
	}

	// Build response with useful info
	resp := map[string]interface{}{
		"id":   entity.ID,
		"name": entity.Name,
		"lat":  entity.Lat,
		"lon":  entity.Lon,
	}

	// Add address and other details from tags
	if data, ok := entity.Data["tags"].(map[string]interface{}); ok {
		if addr := data["addr:street"]; addr != nil {
			if num := data["addr:housenumber"]; num != nil {
				resp["address"] = num.(string) + " " + addr.(string)
			} else {
				resp["address"] = addr
			}
		}
		if postcode := data["addr:postcode"]; postcode != nil {
			resp["postcode"] = postcode
		}
		if hours := data["opening_hours"]; hours != nil {
			resp["hours"] = hours
		}
		if phone := data["phone"]; phone != nil {
			resp["phone"] = phone
		} else if phone := data["contact:phone"]; phone != nil {
			resp["phone"] = phone
		}
		if website := data["website"]; website != nil {
			resp["website"] = website
		}
	}

	// Google Maps link
	resp["map"] = "https://www.google.com/maps/search/?api=1&query=" + 
		strings.ReplaceAll(entity.Name, " ", "+") + 
		"&query_place_id=" + entity.ID

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
