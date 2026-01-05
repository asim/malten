package command

import (
	"fmt"

	"malten.ai/spatial"
)

func init() {
	Register(&Command{
		Name:        "map",
		Description: "View the spatial map",
		Usage:       "/map",
		Handler: func(ctx *Context, args []string) (string, error) {
			return MapInfo(), nil
		},
	})
}

// MapInfo returns information about the map view
func MapInfo() string {
	db := spatial.Get()
	stats := db.Stats()
	
	// Count streets
	streets := db.Query(51.45, -0.35, 50000, spatial.EntityStreet, 1000)
	
	return fmt.Sprintf(`🗺️ Malten Spatial Map

%d agents mapping the world
%d places indexed
%d streets mapped

View at: /map`, stats.Agents, stats.Places, len(streets))
}
