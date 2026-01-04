package command

import (
	"regexp"
	"strings"
)

func init() {
	// Weather - extract from context
	Register(&Command{
		Name:        "weather",
		Description: "Get current weather",
		Emoji:       "⛅",
		LoadingText: "Checking weather...",
		Match: func(input string) (bool, []string) {
			lower := strings.ToLower(input)
			patterns := []string{"weather", "temperature", "how cold", "how hot", "is it cold", "is it hot", "will it rain"}
			for _, p := range patterns {
				if strings.Contains(lower, p) {
					return true, nil
				}
			}
			return false, nil
		},
		Handler: func(ctx *Context, args []string) (string, error) {
			userCtx := GetUserContext(ctx.Session)
			if userCtx == "" {
				return "📍 No location. Enable location to get weather.", nil
			}
			// Extract weather line from context
			// Format: ⛅ 0°C or ☀️ 15°C etc
			re := regexp.MustCompile(`([☀️⛅🌫️🌧️❄️⛈️🌡️]+\s*-?\d+°C)`)
			if match := re.FindString(userCtx); match != "" {
				return match, nil
			}
			return "Weather not available", nil
		},
	})

	// Where am I - extract location
	Register(&Command{
		Name:        "location",
		Description: "Get current location",
		Emoji:       "📍",
		LoadingText: "Getting location...",
		Match: func(input string) (bool, []string) {
			lower := strings.ToLower(input)
			patterns := []string{"where am i", "my location", "what street", "what road"}
			for _, p := range patterns {
				if strings.Contains(lower, p) {
					return true, nil
				}
			}
			return false, nil
		},
		Handler: func(ctx *Context, args []string) (string, error) {
			userCtx := GetUserContext(ctx.Session)
			if userCtx == "" {
				return "📍 No location. Enable location.", nil
			}
			// Extract location line - usually first line starting with 📍
			lines := strings.Split(userCtx, "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "📍") {
					return line, nil
				}
			}
			return "Location not available", nil
		},
	})

	// Next bus
	Register(&Command{
		Name:        "bus",
		Description: "Get next bus times",
		Emoji:       "🚌",
		LoadingText: "Checking bus times...",
		Match: func(input string) (bool, []string) {
			lower := strings.ToLower(input)
			patterns := []string{"next bus", "bus time", "when is the bus", "buses"}
			for _, p := range patterns {
				if strings.Contains(lower, p) {
					return true, nil
				}
			}
			return false, nil
		},
		Handler: func(ctx *Context, args []string) (string, error) {
			userCtx := GetUserContext(ctx.Session)
			if userCtx == "" {
				return "📍 No location. Enable location to get bus times.", nil
			}
			// Extract bus section
			lines := strings.Split(userCtx, "\n")
			var busLines []string
			inBus := false
			for _, line := range lines {
				if strings.HasPrefix(line, "🚏") {
					inBus = true
					busLines = append(busLines, line)
				} else if inBus && strings.HasPrefix(line, "   ") {
					busLines = append(busLines, line)
				} else if inBus {
					break
				}
			}
			if len(busLines) > 0 {
				return strings.Join(busLines, "\n"), nil
			}
			return "No bus times available nearby", nil
		},
	})

	// Prayer times
	Register(&Command{
		Name:        "prayer",
		Description: "Get prayer times",
		Emoji:       "🕌",
		LoadingText: "Getting prayer times...",
		Match: func(input string) (bool, []string) {
			lower := strings.ToLower(input)
			patterns := []string{"prayer", "salah", "fajr", "dhuhr", "asr", "maghrib", "isha", "sunrise"}
			for _, p := range patterns {
				if strings.Contains(lower, p) {
					return true, nil
				}
			}
			return false, nil
		},
		Handler: func(ctx *Context, args []string) (string, error) {
			userCtx := GetUserContext(ctx.Session)
			if userCtx == "" {
				return "📍 No location. Enable location to get prayer times.", nil
			}
			// Extract prayer line
			re := regexp.MustCompile(`🕌[^\n]+`)
			if match := re.FindString(userCtx); match != "" {
				return match, nil
			}
			return "Prayer times not available", nil
		},
	})

	// Quick summary - just "." or "summary"
	Register(&Command{
		Name:        "summary",
		Description: "Quick location summary",
		Emoji:       "📍",
		LoadingText: "Getting summary...",
		Match: func(input string) (bool, []string) {
			trimmed := strings.TrimSpace(input)
			return trimmed == "." || trimmed == ".." || strings.ToLower(trimmed) == "summary", nil
		},
		Handler: func(ctx *Context, args []string) (string, error) {
			userCtx := GetUserContext(ctx.Session)
			if userCtx == "" {
				return "📍 No location. Enable location.", nil
			}
			
			var lines []string
			ctxLines := strings.Split(userCtx, "\n")
			
			// Location
			for _, line := range ctxLines {
				if strings.HasPrefix(line, "📍") {
					lines = append(lines, line)
					break
				}
			}
			
			// Weather + Prayer
			reWeather := regexp.MustCompile(`[☀⛅🌫🌧❄⛈🌡][^·\n]*°C`)
			rePrayer := regexp.MustCompile(`🕌[^\n]+`)
			var weatherPrayer []string
			if match := reWeather.FindString(userCtx); match != "" {
				weatherPrayer = append(weatherPrayer, strings.TrimSpace(match))
			}
			if match := rePrayer.FindString(userCtx); match != "" {
				weatherPrayer = append(weatherPrayer, strings.TrimSpace(match))
			}
			if len(weatherPrayer) > 0 {
				lines = append(lines, strings.Join(weatherPrayer, " "))
			}
			
			// Bus
			rebus := regexp.MustCompile(`(\d+)\s*→\s*([^\s]+)\s+in\s+(\d+)m`)
			if match := rebus.FindStringSubmatch(userCtx); len(match) > 0 {
				lines = append(lines, "🚌 "+match[1]+" to "+match[2]+" in "+match[3]+"m")
			}
			
			// First 2 cafes
			recafe := regexp.MustCompile(`☕\s*\{([^}]+)\}`)
			if match := recafe.FindStringSubmatch(userCtx); len(match) > 0 {
				places := strings.Split(match[1], ";;")
				var names []string
				for i, p := range places {
					if i >= 2 {
						break
					}
					name := strings.Split(p, "|")[0]
					names = append(names, name)
				}
				if len(names) > 0 {
					lines = append(lines, "☕ "+strings.Join(names, ", "))
				}
			}
			
			if len(lines) == 0 {
				return "No context available", nil
			}
			return strings.Join(lines, "\n"), nil
		},
	})

	// Let LLM handle natural language questions like "where is the bus station"
	// It has the context and can answer naturally
}
