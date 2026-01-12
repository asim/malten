package spatial

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"time"
)

// BackfillStreetsForAllPlaces goes through all indexed places and adds streets where missing
// Fetches actual street geometry from OSM, not just names
func BackfillStreetsForAllPlaces(centerLat, centerLon float64, maxRadius float64) int {
	db := Get()
	var totalCount int
	
	// Track streets we've already fetched (by name) to avoid duplicates
	fetchedStreets := make(map[string]bool)
	
	// Work in 500m rings outward
	ringSize := 500.0
	for radius := ringSize; radius <= maxRadius; radius += ringSize {
		innerRadius := radius - ringSize
		
		// Get all places in this ring
		allPlaces := db.Query(centerLat, centerLon, radius, EntityPlace, 1000)
		
		var ringCount int
		for _, place := range allPlaces {
			// Skip if inside inner ring (already processed)
			dist := haversineDistance(centerLat, centerLon, place.Lat, place.Lon)
			if dist < innerRadius {
				continue
			}
			
			// Check if there's already a street near this place (with actual geometry)
			nearbyStreets := db.Query(place.Lat, place.Lon, 30, EntityStreet, 5)
			hasRealStreet := false
			for _, s := range nearbyStreets {
				if sd := s.GetStreetData(); sd != nil && len(sd.Points) >= 2 {
					hasRealStreet = true
					break
				}
			}
			if hasRealStreet {
				continue
			}
			
			// Reverse geocode to get the street name
			streetName := getStreetNameAt(place.Lat, place.Lon)
			if streetName == "" {
				continue
			}
			
			// Skip if we already fetched this street
			streetKey := fmt.Sprintf("%.3f,%.3f:%s", place.Lat, place.Lon, streetName)
			if fetchedStreets[streetKey] {
				continue
			}
			fetchedStreets[streetKey] = true
			
			// Fetch actual street geometry from OSM
			geometry := fetchStreetGeometryFromOSM(place.Lat, place.Lon, streetName)
			if geometry == nil || len(geometry) < 2 {
				log.Printf("[backfill] No geometry found for '%s' near %s", streetName, place.Name)
				continue
			}
			
			// Calculate length
			var length float64
			for i := 0; i < len(geometry)-1; i++ {
				length += haversineDistance(geometry[i][1], geometry[i][0], geometry[i+1][1], geometry[i+1][0])
			}
			
			// Use midpoint for spatial indexing
			midIdx := len(geometry) / 2
			midLon := geometry[midIdx][0]
			midLat := geometry[midIdx][1]
			
			entity := &Entity{
				ID:        GenerateID(EntityStreet, midLat, midLon, streetName),
				Type:      EntityStreet,
				Name:      streetName,
				Lat:       midLat,
				Lon:       midLon,
				Data: &StreetData{
					Points: geometry,
					Length: length,
				},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			db.Insert(entity)
			ringCount++
			
			log.Printf("[backfill] Street '%s' (%d points, %.0fm) near '%s'", streetName, len(geometry), length, place.Name)
			
			// Rate limit - Nominatim + Overpass need breathing room
			time.Sleep(6 * time.Second)
		}
		
		totalCount += ringCount
		log.Printf("[backfill] Ring %.0f-%.0fm: %d streets added (total: %d)", innerRadius, radius, ringCount, totalCount)
	}
	
	return totalCount
}

// getStreetNameAt reverse geocodes to get the street name at a location
func getStreetNameAt(lat, lon float64) string {
	apiURL := fmt.Sprintf("https://nominatim.openstreetmap.org/reverse?lat=%f&lon=%f&format=json&zoom=18", lat, lon)
	
	resp, err := LocationGet(apiURL)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	
	var data struct {
		Address struct {
			Road string `json:"road"`
		} `json:"address"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return ""
	}
	
	return data.Address.Road
}

// fetchStreetGeometryFromOSM fetches the geometry of a street near a location
func fetchStreetGeometryFromOSM(lat, lon float64, streetName string) [][]float64 {
	// Query Overpass for ways with this name near the location
	query := fmt.Sprintf(`
[out:json][timeout:10];
way["name"="%s"](around:200,%f,%f);
out geom;
`, streetName, lat, lon)
	
	apiURL := "https://overpass-api.de/api/interpreter?data=" + url.QueryEscape(query)
	
	resp, err := OSMGet(apiURL)
	if err != nil {
		log.Printf("[backfill] Overpass error: %v", err)
		return nil
	}
	defer resp.Body.Close()
	
	var result struct {
		Elements []struct {
			Type     string `json:"type"`
			Geometry []struct {
				Lat float64 `json:"lat"`
				Lon float64 `json:"lon"`
			} `json:"geometry"`
		} `json:"elements"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("[backfill] Overpass decode error: %v", err)
		return nil
	}
	
	// Find the longest way (main street segment)
	var bestGeometry [][]float64
	for _, el := range result.Elements {
		if el.Type != "way" || len(el.Geometry) < 2 {
			continue
		}
		
		geom := make([][]float64, len(el.Geometry))
		for i, pt := range el.Geometry {
			geom[i] = []float64{pt.Lon, pt.Lat}
		}
		
		if len(geom) > len(bestGeometry) {
			bestGeometry = geom
		}
	}
	
	return bestGeometry
}

// CleanupBrokenStreets removes street entries with less than 2 points (incomplete geometry)
func CleanupBrokenStreets() int {
	db := Get()
	db.mu.Lock()
	defer db.mu.Unlock()

	var toDelete []string
	for id, point := range db.entities {
		entity, ok := point.Data().(*Entity)
		if !ok || entity.Type != EntityStreet {
			continue
		}

		sd := entity.GetStreetData()
		if sd == nil || len(sd.Points) < 2 {
			toDelete = append(toDelete, id)
		}
	}

	for _, id := range toDelete {
		if point, ok := db.entities[id]; ok {
			db.tree.Remove(point)
			delete(db.entities, id)
			db.store.Delete(id)
		}
	}

	if len(toDelete) > 0 {
		log.Printf("[cleanup] Removed %d broken street entries (< 2 points)", len(toDelete))
	}

	return len(toDelete)
}
