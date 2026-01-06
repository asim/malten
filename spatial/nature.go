package spatial

import (
	"fmt"
	"math/rand"
	"time"
)

// NatureReminder represents a reminder to look at the natural world
type NatureReminder struct {
	Image   string // URL to image
	Caption string // Short caption
	Verse   string // Optional verse/reflection
	Type    string // stars, mountains, ocean, sunrise, etc.
}

// Nature reminders by type
var natureReminders = map[string][]NatureReminder{
	"stars": {
		{
			Caption: "The stars are out tonight",
			Verse:   "And it is He who placed for you the stars that you may be guided by them through the darknesses of the land and sea. (6:97)",
			Type:    "stars",
		},
		{
			Caption: "Look up",
			Verse:   "Do they not look at the sky above them - how We structured it and adorned it? (50:6)",
			Type:    "stars",
		},
		{
			Caption: "Clear skies tonight",
			Type:    "stars",
		},
	},
	"moon": {
		{
			Caption: "The moon is bright tonight",
			Verse:   "It is He who made the sun a shining light and the moon a derived light and determined for it phases. (10:5)",
			Type:    "moon",
		},
		{
			Caption: "Moonlight",
			Type:    "moon",
		},
	},
	"sunrise": {
		{
			Caption: "Sunrise soon",
			Verse:   "By the morning brightness. (93:1)",
			Type:    "sunrise",
		},
		{
			Caption: "A new day begins",
			Type:    "sunrise",
		},
	},
	"sunset": {
		{
			Caption: "Sunset",
			Verse:   "By the declining day. (103:1)",
			Type:    "sunset",
		},
	},
	"mountains": {
		{
			Caption: "Mountains stand firm",
			Verse:   "And the mountains as stakes. (78:7)",
			Type:    "mountains",
		},
	},
	"ocean": {
		{
			Caption: "The vast ocean",
			Verse:   "And it is He who subjected the sea for you to eat from it tender meat. (16:14)",
			Type:    "ocean",
		},
	},
	"rain": {
		{
			Caption: "Rain brings life",
			Verse:   "And We have sent down blessed rain from the sky and made grow thereby gardens. (50:9)",
			Type:    "rain",
		},
	},
	"snow": {
		{
			Caption: "Snow falls silently",
			Type:    "snow",
		},
	},
}

// GetNatureReminder returns an appropriate nature reminder based on conditions
// Returns nil if no reminder is appropriate right now
func GetNatureReminder(lat, lon float64, weather *WeatherData, sunTimes *SunTimes) *NatureReminder {
	now := time.Now()
	hour := now.Hour()
	
	// Don't spam - only show occasionally
	// This should be called at most once per session per day
	
	var reminderType string
	
	// Night time (after 9pm, before 6am) - stars or moon
	if hour >= 21 || hour < 6 {
		// Check if clear skies (weather code 0-3 is clear/partly cloudy)
		if weather != nil && weather.WeatherCode <= 3 {
			if rand.Float32() < 0.5 {
				reminderType = "stars"
			} else {
				reminderType = "moon"
			}
		}
	}
	
	// Around sunrise (5-7am)
	if hour >= 5 && hour < 7 {
		reminderType = "sunrise"
	}
	
	// Around sunset (check actual sunset time if available)
	if sunTimes != nil {
		sunsetHour := sunTimes.Sunset.Hour()
		if hour == sunsetHour || hour == sunsetHour-1 {
			reminderType = "sunset"
		}
	} else if hour >= 16 && hour < 18 {
		// Default sunset window
		reminderType = "sunset"
	}
	
	// Weather-based
	if weather != nil {
		// Raining (codes 51-67, 80-82)
		if weather.WeatherCode >= 51 && weather.WeatherCode <= 67 ||
			weather.WeatherCode >= 80 && weather.WeatherCode <= 82 {
			if rand.Float32() < 0.3 { // Only sometimes
				reminderType = "rain"
			}
		}
		// Snowing (codes 71-77, 85-86)
		if weather.WeatherCode >= 71 && weather.WeatherCode <= 77 ||
			weather.WeatherCode >= 85 && weather.WeatherCode <= 86 {
			reminderType = "snow"
		}
	}
	
	if reminderType == "" {
		return nil
	}
	
	reminders, ok := natureReminders[reminderType]
	if !ok || len(reminders) == 0 {
		return nil
	}
	
	// Pick a random one
	reminder := reminders[rand.Intn(len(reminders))]
	return &reminder
}

// FormatNatureReminder formats a nature reminder for display
func FormatNatureReminder(r *NatureReminder) string {
	if r == nil {
		return ""
	}
	
	// Emoji based on type
	emoji := "🌿"
	switch r.Type {
	case "stars":
		emoji = "✨"
	case "moon":
		emoji = "🌙"
	case "sunrise":
		emoji = "🌅"
	case "sunset":
		emoji = "🌇"
	case "mountains":
		emoji = "🏔️"
	case "ocean":
		emoji = "🌊"
	case "rain":
		emoji = "🌧️"
	case "snow":
		emoji = "❄️"
	}
	
	result := fmt.Sprintf("%s %s", emoji, r.Caption)
	if r.Verse != "" {
		result += "\n\n" + r.Verse
	}
	return result
}

// SunTimes holds sunrise/sunset times
type SunTimes struct {
	Sunrise time.Time
	Sunset  time.Time
}
