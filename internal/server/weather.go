package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// weather.go adds current conditions from Open-Meteo — free, keyless, worldwide,
// JSON (so no new dependency). It's folded into the "around me" snapshot so the
// timeline and the agent both see it.

var weatherClient = &http.Client{Timeout: 6 * time.Second}

// Weather is the current conditions at a point.
type Weather struct {
	TempC   float64 `json:"temp_c"`
	Code    int     `json:"code"`
	Text    string  `json:"text"`
	WindKph float64 `json:"wind_kph"`
	IsDay   bool    `json:"is_day"`
}

// fetchWeather returns current conditions for a lat/lng, or nil on any error.
func fetchWeather(ctx context.Context, lat, lng float64) *Weather {
	u := fmt.Sprintf("https://api.open-meteo.com/v1/forecast?latitude=%.4f&longitude=%.4f"+
		"&current=temperature_2m,weather_code,wind_speed_10m,is_day&wind_speed_unit=kmh&timezone=auto", lat, lng)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil
	}
	resp, err := weatherClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}
	var raw struct {
		Current struct {
			Temp float64 `json:"temperature_2m"`
			Code int     `json:"weather_code"`
			Wind float64 `json:"wind_speed_10m"`
			Day  int     `json:"is_day"`
		} `json:"current"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil
	}
	return &Weather{
		TempC:   raw.Current.Temp,
		Code:    raw.Current.Code,
		Text:    wmoText(raw.Current.Code),
		WindKph: raw.Current.Wind,
		IsDay:   raw.Current.Day == 1,
	}
}

// wmoText maps a WMO weather code to a short description.
func wmoText(code int) string {
	switch {
	case code == 0:
		return "Clear"
	case code == 1:
		return "Mainly clear"
	case code == 2:
		return "Partly cloudy"
	case code == 3:
		return "Overcast"
	case code == 45 || code == 48:
		return "Fog"
	case code >= 51 && code <= 57:
		return "Drizzle"
	case code >= 61 && code <= 67:
		return "Rain"
	case code >= 71 && code <= 77:
		return "Snow"
	case code >= 80 && code <= 82:
		return "Showers"
	case code == 85 || code == 86:
		return "Snow showers"
	case code >= 95:
		return "Thunderstorm"
	default:
		return "—"
	}
}
