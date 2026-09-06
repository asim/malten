package nature

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/asim/malten/agent"
	"math"
	"time"
)

type conditions struct {
	Time        int64
	Temperature float64 `json:"temperature_2m"`
	Code        int     `json:"weather_code"`
	Rain        float64 `json:"precipitation"`
	Wind        float64 `json:"wind_speed_10m"`
	Day         int     `json:"is_day"`
}
type weatherPlace struct {
	Place, Name, Timezone string
	Data                  struct {
		Current conditions
		Daily   struct{ Sunrise, Sunset []int64 }
	}
}

func decide(_ context.Context, v agent.View) (agent.Decision, error) {
	var current struct{ Places []weatherPlace }
	id := ""
	for i := len(v.Records) - 1; i >= 0; i-- {
		r := v.Records[i]
		if r.Kind == "source" {
			if err := json.Unmarshal(r.Data, &current); err != nil {
				return agent.Decision{}, err
			}
			id = r.ID
			break
		}
	}
	summary, _ := json.Marshal(current)
	d := agent.Decision{Summary: string(summary)}
	if v.SourceUnavailable {
		return d, nil
	}
	for _, p := range current.Places {
		now := p.Data.Current
		if v.Now.Sub(time.Unix(now.Time, 0)) > time.Hour {
			continue
		}
		changed := true
		for i := len(v.Records) - 1; i >= 0; i-- {
			r := v.Records[i]
			if r.Status != "sent" || r.Action == nil || r.Action.Place != p.Place {
				continue
			}
			var old struct{ Places []weatherPlace }
			if json.Unmarshal([]byte(r.Summary), &old) == nil {
				for _, before := range old.Places {
					if before.Place == p.Place {
						was := before.Data.Current
						changed = now.Code != was.Code || now.Day != was.Day || (now.Rain > 0) != (was.Rain > 0) || math.Abs(now.Temperature-was.Temperature) >= 3
					}
				}
			}
			break
		}
		if !changed {
			continue
		}
		loc, err := time.LoadLocation(p.Timezone)
		if err != nil {
			return d, err
		}
		if len(p.Data.Daily.Sunrise) != 1 || len(p.Data.Daily.Sunset) != 1 {
			continue
		}
		text := fmt.Sprintf("%s · %s\n%.0f°C · %s\nSunrise %s · Sunset %s\nWeather estimate · https://open-meteo.com/", p.Name, time.Unix(now.Time, 0).In(loc).Format("2 Jan · 15:04 MST"), now.Temperature, description(now.Code), time.Unix(p.Data.Daily.Sunrise[0], 0).In(loc).Format("15:04"), time.Unix(p.Data.Daily.Sunset[0], 0).In(loc).Format("15:04"))
		d.Action = &agent.Action{Stream: "nature", Place: p.Place, Text: text}
		d.Evidence = []string{id}
		return d, nil
	}
	return d, nil
}
func description(code int) string {
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
		return "Rain showers"
	case code == 85 || code == 86:
		return "Snow showers"
	case code >= 95:
		return "Thunderstorms"
	default:
		return "Conditions unavailable"
	}
}
