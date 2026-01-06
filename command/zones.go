package command

import (
	"fmt"
	"sort"
	"strings"

	"malten.ai/spatial"
)

func init() {
	Register(&Command{
		Name:        "zones",
		Description: "Show coverage analysis and dead zones",
		Usage:       "/zones",
		Handler:     handleZones,
	})
}

func handleZones(ctx *Context, args []string) (string, error) {
	db := spatial.Get()
	agents := db.ListAgents()
	
	if len(agents) == 0 {
		return "No agents found", nil
	}
	
	// Count streets near each agent
	type agentCoverage struct {
		Name    string
		Lat     float64
		Lon     float64
		Streets int
	}
	
	var coverage []agentCoverage
	for _, agent := range agents {
		streets := db.Query(agent.Lat, agent.Lon, 1000, spatial.EntityStreet, 100)
		coverage = append(coverage, agentCoverage{
			Name:    agent.Name,
			Lat:     agent.Lat,
			Lon:     agent.Lon,
			Streets: len(streets),
		})
	}
	
	// Sort by coverage
	sort.Slice(coverage, func(i, j int) bool {
		return coverage[i].Streets > coverage[j].Streets
	})
	
	var lines []string
	lines = append(lines, "📊 **Coverage Analysis**\n")
	
	// Well connected (10+ streets)
	var wellConnected []string
	for _, c := range coverage {
		if c.Streets >= 10 {
			wellConnected = append(wellConnected, fmt.Sprintf("%s (%d)", c.Name, c.Streets))
		}
	}
	if len(wellConnected) > 0 {
		lines = append(lines, "**Well connected:**")
		lines = append(lines, strings.Join(wellConnected[:min(5, len(wellConnected))], ", "))
		lines = append(lines, "")
	}
	
	// Sparse (1-9 streets)
	var sparse []string
	for _, c := range coverage {
		if c.Streets >= 1 && c.Streets < 10 {
			sparse = append(sparse, fmt.Sprintf("%s (%d)", c.Name, c.Streets))
		}
	}
	if len(sparse) > 0 {
		lines = append(lines, "**Sparse:**")
		lines = append(lines, strings.Join(sparse[:min(5, len(sparse))], ", "))
		lines = append(lines, "")
	}
	
	// Isolated (0 streets)
	var isolated []string
	for _, c := range coverage {
		if c.Streets == 0 {
			isolated = append(isolated, c.Name)
		}
	}
	if len(isolated) > 0 {
		lines = append(lines, "**Isolated (no streets):**")
		lines = append(lines, strings.Join(isolated[:min(5, len(isolated))], ", "))
		lines = append(lines, "")
	}
	
	// Find gaps between agents (pairs that should be connected but aren't)
	lines = append(lines, "**Dead zones** (gaps to fill):")
	
	type gap struct {
		From     string
		To       string
		Distance float64
	}
	var gaps []gap
	
	for i, a1 := range coverage {
		for j, a2 := range coverage {
			if i >= j {
				continue
			}
			
			// Only consider nearby agents (< 10km)
			dist := spatial.DistanceMeters(a1.Lat, a1.Lon, a2.Lat, a2.Lon)
			if dist > 10000 || dist < 500 {
				continue
			}
			
			// Check if connected via streets
			if !areAreasConnected(db, a1.Lat, a1.Lon, a2.Lat, a2.Lon) {
				gaps = append(gaps, gap{
					From:     a1.Name,
					To:       a2.Name,
					Distance: dist,
				})
			}
		}
	}
	
	// Sort by distance
	sort.Slice(gaps, func(i, j int) bool {
		return gaps[i].Distance < gaps[j].Distance
	})
	
	if len(gaps) == 0 {
		lines = append(lines, "None found - good coverage!")
	} else {
		for i, g := range gaps {
			if i >= 5 {
				lines = append(lines, fmt.Sprintf("... and %d more", len(gaps)-5))
				break
			}
			lines = append(lines, fmt.Sprintf("• %s ↔ %s (%.1fkm)", g.From, g.To, g.Distance/1000))
		}
	}
	
	return strings.Join(lines, "\n"), nil
}

// areAreasConnected checks if two areas have streets connecting them
func areAreasConnected(db *spatial.DB, lat1, lon1, lat2, lon2 float64) bool {
	// Check if any street from area1 ends near area2
	streets := db.Query(lat1, lon1, 1500, spatial.EntityStreet, 50)
	
	for _, street := range streets {
		sd := street.GetStreetData()
		if sd == nil || len(sd.Points) < 2 {
			continue
		}
		
		// Check if street ends near area2
		endLon, endLat := sd.Points[len(sd.Points)-1][0], sd.Points[len(sd.Points)-1][1]
		endDist := spatial.DistanceMeters(endLat, endLon, lat2, lon2)
		if endDist < 1000 {
			return true
		}
		
		// Check start too
		startLon, startLat := sd.Points[0][0], sd.Points[0][1]
		startDist := spatial.DistanceMeters(startLat, startLon, lat2, lon2)
		if startDist < 1000 {
			return true
		}
	}
	
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
