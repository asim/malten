// Package nature maintains current weather and daylight context for known places.
package nature

import (
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/asim/malten/agent"
	"net/url"
	"os"
	"strings"
	"time"
)

//go:embed photos/*.jpg
var photos embed.FS

type place struct{ Tag, Name, Latitude, Longitude, Credit, Source string }

func New() agent.Agent {
	return agent.Agent{
		DisplayName: "Nature",
		Name:        "nature",
		Objective:   "Weather now. Maintain current weather and daylight context for each supplied city using Open-Meteo. Compare previous conditions: precipitation beginning/ending, weather code or day/night changing, or a temperature change of at least 3 C can warrant an update. Small fluctuations and timestamps alone do not. Initial context may warrant one concise update in nature. Publish in a city only when a recent observation there makes it relevant. Set Action.Place to its exact city tag. Describe current values as weather model estimates, include the local time and Open-Meteo attribution, never imply live measurements or turn an illustrative photo into a live view. Do not extrapolate to regions.",
		Read:        Read, Decide: decide, Check: check, Media: media,
	}
}
func Read(ctx context.Context, now time.Time) (json.RawMessage, error) {
	var lat, lon []string
	for _, p := range places {
		lat = append(lat, p.Latitude)
		lon = append(lon, p.Longitude)
	}
	q := url.Values{"latitude": {strings.Join(lat, ",")}, "longitude": {strings.Join(lon, ",")}, "timezone": {"auto"}, "timeformat": {"unixtime"}, "current": {"temperature_2m,precipitation,weather_code,wind_speed_10m,is_day"}, "daily": {"sunrise,sunset"}, "forecast_days": {"1"}}
	base := "https://api.open-meteo.com/v1/forecast"
	if key := os.Getenv("OPEN_METEO_API_KEY"); key != "" {
		base = "https://customer-api.open-meteo.com/v1/forecast"
		q.Set("apikey", key)
	}
	raw, err := agent.ReadJSON(ctx, "GET", base+"?"+q.Encode(), "")
	if err != nil {
		return nil, errors.New("weather source unavailable")
	}
	var data []json.RawMessage
	if json.Unmarshal(raw, &data) != nil || len(data) != len(places) {
		return nil, errors.New("incomplete weather locations")
	}
	type entry struct {
		Place, Name, Timezone string
		Data                  json.RawMessage
	}
	entries := []entry{}
	for i, r := range data {
		var d struct {
			Timezone string
			Current  struct {
				Time        int64
				Temperature *float64 `json:"temperature_2m"`
				Code        *int     `json:"weather_code"`
			}
			Daily struct{ Sunrise, Sunset []int64 }
		}
		if json.Unmarshal(r, &d) != nil || d.Current.Temperature == nil || d.Current.Code == nil || d.Current.Time > now.Add(15*time.Minute).Unix() || now.Sub(time.Unix(d.Current.Time, 0)) > time.Hour || len(d.Daily.Sunrise) != 1 || len(d.Daily.Sunset) != 1 || d.Daily.Sunset[0] <= d.Daily.Sunrise[0] {
			return nil, errors.New("stale or incomplete weather")
		}
		if d.Timezone != agent.Cities[i].Timezone {
			return nil, errors.New("unexpected weather timezone")
		}
		entries = append(entries, entry{places[i].Tag, places[i].Name, d.Timezone, r})
	}
	return json.Marshal(struct {
		Source, Meaning string
		Places          []entry
	}{"https://open-meteo.com/", "Current weather model estimates and calculated solar times. Epoch times are UTC; use each place's timezone for local time.", entries})
}
func check(v agent.View, d agent.Decision) bool {
	if v.SourceUnavailable {
		return false
	}
	for _, p := range places {
		if d.Action.Place == p.Tag && (d.Action.Stream == "nature" || d.Action.Stream == p.Tag) && strings.Contains(d.Action.Text, "https://open-meteo.com") {
			return true
		}
	}
	return false
}
func media(a agent.Action) (string, string) {
	for _, p := range places {
		if p.Tag == a.Place {
			raw, err := photos.ReadFile("photos/" + p.Tag + ".jpg")
			if err != nil {
				return "", ""
			}
			return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(raw), "Illustrative photo, not live · " + p.Credit + " (CC0)\n" + p.Source
		}
	}
	return "", ""
}
