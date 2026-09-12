package nature

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/asim/malten/agent"
	"strings"
	"time"
)

func muLookups() []agent.Lookup {
	if !agent.MuAvailable() {
		return nil
	}
	return []agent.Lookup{
		{Name: "locate_place", Description: "Resolve a town or place explicitly named in the question to coordinates. Never infer where the person is from an image or unrelated context.", InputSchema: json.RawMessage(`{"type":"object","properties":{"address":{"type":"string","maxLength":160}},"required":["address"],"additionalProperties":false}`), Read: func(ctx context.Context, raw json.RawMessage) ([]agent.Source, error) {
			var q struct{ Address string }
			if agent.Decode(raw, &q) != nil || strings.TrimSpace(q.Address) == "" || len([]rune(q.Address)) > 160 {
				return nil, errors.New("invalid place")
			}
			text, err := agent.MuCall(ctx, "places_geocode", map[string]string{"address": q.Address})
			return muSource("Place lookup via Micro", "https://micro.mu/places", text, err)
		}},
		{Name: "weather_forecast", Description: "Read weather and daylight at coordinates supplied in the question or returned by locate_place. Preserve provider dates and uncertainty. Do not guess coordinates or claim current weather describes a past photo.", InputSchema: json.RawMessage(`{"type":"object","properties":{"lat":{"type":"number","minimum":-90,"maximum":90},"lon":{"type":"number","minimum":-180,"maximum":180}},"required":["lat","lon"],"additionalProperties":false}`), Read: func(ctx context.Context, raw json.RawMessage) ([]agent.Source, error) {
			var q struct{ Lat, Lon *float64 }
			if agent.Decode(raw, &q) != nil || q.Lat == nil || q.Lon == nil || *q.Lat < -90 || *q.Lat > 90 || *q.Lon < -180 || *q.Lon > 180 {
				return nil, errors.New("invalid coordinates")
			}
			text, err := agent.MuCall(ctx, "weather_forecast", map[string]float64{"lat": *q.Lat, "lon": *q.Lon})
			return muSource(fmt.Sprintf("Weather via Micro · %.4f, %.4f", *q.Lat, *q.Lon), "https://micro.mu/weather", text, err)
		}},
	}
}
func muSource(title, address, text string, err error) ([]agent.Source, error) {
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(text) == "" || len(text) > 12000 {
		return nil, errors.New("invalid Mu source")
	}
	return []agent.Source{agent.NewSource(title, address, "Retrieved "+time.Now().UTC().Format(time.RFC3339)+"\n"+text, false)}, nil
}
