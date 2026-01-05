package command

import (
	"fmt"

	"malten.ai/spatial"
)

func init() {
	Register(&Command{
		Name:        "streets",
		Description: "Index streets in your area",
		Usage:       "/streets",
		Handler: func(ctx *Context, args []string) (string, error) {
			if !ctx.HasLocation() {
				return "Location needed to index streets", nil
			}
			return IndexStreets(ctx.Lat, ctx.Lon), nil
		},
	})
}

// IndexStreets triggers street indexing for the area
func IndexStreets(lat, lon float64) string {
	db := spatial.Get()
	
	// Find or create agent for this area
	agent := db.FindOrCreateAgent(lat, lon)
	if agent == nil {
		return "Could not find agent for this area"
	}
	
	// Quick index - just 5 routes for immediate feedback
	count := spatial.IndexStreetsAroundAgent(agent, 5)
	
	// Start background indexing for the rest
	spatial.IndexStreetsAsync(agent)
	
	if count == 0 {
		return fmt.Sprintf("🛣️ Starting street indexing around %s...\nCheck /map in a minute to see progress.", agent.Name)
	}
	
	return fmt.Sprintf("🛣️ Indexed %d street routes around %s\nMore indexing in background...", count, agent.Name)
}
