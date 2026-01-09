package command

import (
	"strings"

	"malten.ai/spatial"
)

func init() {
	Register(&Command{
		Name:        "nature",
		Description: "Get a nature image",
		Usage:       "/nature [type]",
		Handler:     handleNature,
	})
}

func handleNature(ctx *Context, args []string) (string, error) {
	var natureType string
	
	if len(args) > 0 {
		// User specified a type - use it directly
		natureType = strings.ToLower(strings.Join(args, " "))
	} else {
		// No type specified - pick based on time of day or weather
		if ctx.HasLocation() {
			db := spatial.Get()
			weatherEntities := db.Query(ctx.Lat, ctx.Lon, 10000, spatial.EntityWeather, 1)
			
			var weather *spatial.WeatherData
			if len(weatherEntities) > 0 {
				weather = weatherEntities[0].GetWeatherData()
			}
			
			reminder := spatial.GetNatureReminder(ctx.Lat, ctx.Lon, weather, nil)
			if reminder != nil {
				natureType = reminder.Type
			}
		}
		
		// Default fallback
		if natureType == "" {
			natureType = "stars"
		}
	}
	
	// Fetch image
	image := spatial.FetchNatureImage(natureType)
	if image == "" {
		return "❌ Unknown type: " + natureType, nil
	}
	
	// Get a caption for the type
	caption := getNatureCaption(natureType)
	
	// Return as HTML with clickable image and caption
	return `<img src="` + image + `" class="reminder-image" onclick="viewReminderImage(this.src)" alt="` + natureType + `">` +
		`<div class="nature-caption">` + caption + `</div>`, nil
}

// getNatureCaption returns a reflective caption for the nature type
func getNatureCaption(natureType string) string {
	captions := map[string]string{
		"stars":     "✨ The night sky",
		"moon":      "🌙 The moon",
		"sunrise":   "🌅 A new day begins",
		"sunset":    "🌇 Day turns to night",
		"morning":   "☀️ Good morning",
		"evening":   "🌆 Good evening",
		"night":     "🌃 The night",
		"rain":      "🌧️ Rain",
		"snow":      "❄️ Snow",
		"clouds":    "☁️ Clouds",
		"fog":       "🌫️ Fog",
		"beach":     "🏖️ The shore",
		"mountains": "⛰️ Mountains",
		"forest":    "🌲 The forest",
		"flowers":   "🌺 Flowers",
		"autumn":    "🍂 Autumn",
		"spring":    "🌸 Spring",
		"winter":    "❄️ Winter",
		"desert":    "🏜️ The desert",
		"ocean":     "🌊 The ocean",
		"river":     "🌊 A river",
		"lake":      "💧 A lake",
	}
	
	if caption, ok := captions[natureType]; ok {
		return caption
	}
	return "🌿 " + natureType
}
