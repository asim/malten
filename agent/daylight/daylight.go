// Package daylight shares calculated solar times with illustrative city photos.
package daylight

import (
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/asim/malten/agent"
)

//go:embed photos/*.jpg
var photos embed.FS

var Streams = []agent.Stream{{Tag: "daylight"}}

type place struct{ Tag, Name, Latitude, Longitude, Credit, Source string }

type solarDay struct {
	Date            string
	Sunrise, Sunset time.Time
}
type Agent struct {
	sync.Mutex
	days map[string]solarDay
}

func New() *Agent { return &Agent{days: map[string]solarDay{}} }

// Run rotates cities in #daylight. City streams use Fetch with their own clock.
func (a *Agent) Run(ctx context.Context, publish agent.PublishPhoto) {
	agent.RunSource(ctx, "daylight", func(ctx context.Context, _ time.Time) (agent.Post, error) {
		city := agent.Cities[(time.Now().Unix()/3600)%int64(len(agent.Cities))]
		loc, err := time.LoadLocation(city.Timezone)
		if err != nil {
			return agent.Post{}, err
		}
		return a.Fetch(ctx, time.Now().In(loc))
	}, publish)
}

func (a *Agent) Fetch(ctx context.Context, local time.Time) (agent.Post, error) {
	// A broad region has no single sunrise or sunset.
	if local.IsZero() {
		return agent.Post{}, nil
	}
	tag := ""
	for _, city := range agent.Cities {
		if city.Timezone == local.Location().String() {
			tag = city.Tag
			break
		}
	}
	for _, p := range places {
		if p.Tag != tag {
			continue
		}
		day, err := a.day(ctx, p, local)
		if err != nil {
			return agent.Post{}, err
		}
		raw, err := photos.ReadFile("photos/" + tag + ".jpg")
		if err != nil {
			return agent.Post{}, err
		}
		return agent.Post{Text: describe(p, day, local), Name: "Daylight", Photo: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(raw)}, nil
	}
	return agent.Post{}, nil
}

func (a *Agent) day(ctx context.Context, p place, local time.Time) (solarDay, error) {
	// Keep one day per city. Concurrent loops share the same request and cache.
	a.Lock()
	defer a.Unlock()
	date := local.Format("2006-01-02")
	if day, ok := a.days[p.Tag]; ok && day.Date == date {
		return day, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	query := url.Values{"lat": {p.Latitude}, "lng": {p.Longitude}, "date": {date}, "formatted": {"0"}, "tzid": {local.Location().String()}}
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.sunrise-sunset.org/json?"+query.Encode(), nil)
	if err != nil {
		return solarDay{}, err
	}
	req.Header.Set("User-Agent", "Malten (https://github.com/asim/malten)")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return solarDay{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return solarDay{}, fmt.Errorf("daylight status %d", res.StatusCode)
	}
	day, err := readDay(io.LimitReader(res.Body, 16384), local)
	if err == nil {
		a.days[p.Tag] = day
	}
	return day, err
}

func readDay(r io.Reader, local time.Time) (solarDay, error) {
	var response struct {
		Status  string
		Results struct{ Sunrise, Sunset time.Time }
	}
	if err := json.NewDecoder(r).Decode(&response); err != nil {
		return solarDay{}, err
	}
	rise, set := response.Results.Sunrise.In(local.Location()), response.Results.Sunset.In(local.Location())
	date := local.Format("2006-01-02")
	if response.Status != "OK" || rise.IsZero() || set.IsZero() || !set.After(rise) || set.Sub(rise) >= 24*time.Hour || rise.Format("2006-01-02") != date || set.Format("2006-01-02") != date {
		return solarDay{}, fmt.Errorf("unavailable solar times for %s", date)
	}
	return solarDay{Date: date, Sunrise: rise, Sunset: set}, nil
}

func describe(p place, day solarDay, local time.Time) string {
	phase := "Daylight"
	if local.Before(day.Sunrise) {
		phase = "Before sunrise"
	} else if !local.Before(day.Sunset) {
		phase = "After sunset"
	}
	return fmt.Sprintf("%s · %s\n%s. Sunrise %s · Sunset %s\nCalculated solar times: https://sunrise-sunset.org\n\nIllustrative photo, not a live view · %s (CC0)\n%s", p.Name, local.Format("2 Jan 2006 · 15:04 MST"), phase, day.Sunrise.Format("15:04"), day.Sunset.Format("15:04"), p.Credit, p.Source)
}
