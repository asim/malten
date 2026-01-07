package spatial

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/url"
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
	"evening": {
		{
			Caption: "The day draws to a close",
			Verse:   "And by the night when it covers. (92:1)",
			Type:    "evening",
		},
		{
			Caption: "Evening light",
			Type:    "evening",
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
		// Default sunset window (winter UK ~16:00-17:00)
		reminderType = "sunset"
	}
	
	// Evening/dusk (18:00-21:00)
	if reminderType == "" && hour >= 18 && hour < 21 {
		reminderType = "evening"
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
	
	var result string
	
	// Include image if available
	if r.Image != "" {
		result = fmt.Sprintf("![%s](%s)\n\n", r.Type, r.Image)
	}
	
	result += r.Caption
	if r.Verse != "" {
		result += "\n\n_" + r.Verse + "_"
	}
	return result
}

// SunTimes holds sunrise/sunset times
type SunTimes struct {
	Sunrise time.Time
	Sunset  time.Time
}

// Wikimedia category mappings for each nature type
var wikimediaCategories = map[string]string{
	"stars":     "Starry_night_sky",
	"moon":      "Photographs_of_the_Moon",
	"sunrise":   "Sunrises",
	"sunset":    "Sunsets",
	"mountains": "Mountain_landscapes",
	"ocean":     "Seascapes",
	"rain":      "Rain",
	"snow":      "Snow_landscapes",
	"evening":   "Dusk",
}

// FetchNatureImage fetches a random image URL from Wikimedia Commons
func FetchNatureImage(natureType string) string {
	category, ok := wikimediaCategories[natureType]
	if !ok {
		return ""
	}
	
	// Query Wikimedia Commons API using the rate-limited External client
	apiURL := fmt.Sprintf(
		"https://commons.wikimedia.org/w/api.php?action=query&generator=categorymembers&gcmtitle=Category:%s&gcmtype=file&gcmlimit=20&prop=imageinfo&iiprop=url&iiurlwidth=800&format=json",
		url.QueryEscape(category),
	)
	
	resp, err := External.Get("wikimedia", apiURL)
	if err != nil {
		log.Printf("[nature] Failed to fetch image: %v", err)
		return ""
	}
	defer resp.Body.Close()
	
	var result struct {
		Query struct {
			Pages map[string]struct {
				ImageInfo []struct {
					ThumbURL string `json:"thumburl"`
				} `json:"imageinfo"`
			} `json:"pages"`
		} `json:"query"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("[nature] Failed to parse image response: %v", err)
		return ""
	}
	
	// Collect all image URLs
	var urls []string
	for _, page := range result.Query.Pages {
		for _, info := range page.ImageInfo {
			if info.ThumbURL != "" {
				urls = append(urls, info.ThumbURL)
			}
		}
	}
	
	if len(urls) == 0 {
		return ""
	}
	
	// Return a random one
	return urls[rand.Intn(len(urls))]
}
