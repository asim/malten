package command

import (
	"malten.ai/spatial"
)

func init() {
	Register(&Command{
		Name:        "nature",
		Description: "Get a nature reminder",
		Usage:       "/nature",
		Handler:     handleNature,
	})
}

func handleNature(ctx *Context, args []string) (string, error) {
	if !ctx.HasLocation() {
		return "📍 Enable location for nature reminders.", nil
	}
	
	// Get weather for this location
	db := spatial.Get()
	weatherEntities := db.Query(ctx.Lat, ctx.Lon, 10000, spatial.EntityWeather, 1)
	
	var weather *spatial.WeatherData
	if len(weatherEntities) > 0 {
		weather = weatherEntities[0].GetWeatherData()
	}
	
	// For now, no sun times - could add later
	reminder := spatial.GetNatureReminder(ctx.Lat, ctx.Lon, weather, nil)
	if reminder == nil {
		// Return a default one
		reminder = &spatial.NatureReminder{
			Caption: "Step outside for a moment",
			Type:    "stars", // Default to stars for image
		}
	}
	
	// Fetch an image for this nature type
	reminder.Image = spatial.FetchNatureImage(reminder.Type)
	
	return spatial.FormatNatureReminder(reminder), nil
}
