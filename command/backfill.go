package command

import (
	"fmt"
	"strconv"

	"malten.ai/spatial"
)

func init() {
	Register(&Command{
		Name:        "backfill",
		Description: "Backfill streets for places without them",
		Usage:       "/backfill [radius_meters]",
		Handler:     handleBackfillStreets,
	})
}

func handleBackfillStreets(ctx *Context, args []string) (string, error) {
	if !ctx.HasLocation() {
		return "📍 Need location to backfill streets.", nil
	}
	
	radius := 5000.0 // 5km default
	if len(args) > 0 {
		if r, err := strconv.ParseFloat(args[0], 64); err == nil {
			radius = r
		}
	}
	
	// Run in background
	go func() {
		spatial.BackfillStreetsForAllPlaces(ctx.Lat, ctx.Lon, radius)
	}()
	
	return fmt.Sprintf("🗺️ Starting street backfill from %.4f,%.4f radius %.0fm\nCheck logs for progress.", ctx.Lat, ctx.Lon, radius), nil
}
