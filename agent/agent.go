// Package agent contains server-owned loops with specific objectives.
package agent

import (
	"context"
	"log"
	"time"
	_ "time/tzdata"
)

// Stream describes an agent's public stream for discovery.
type Stream struct {
	Tag string
}

// PublishPhoto sends agent text and optional photos through moderation.
type PublishPhoto func(context.Context, string, string, string, string, ...string) error

type Post struct{ Text, Photo, Name string }
type Source func(context.Context, time.Time) (Post, error)

// Regions span timezones; cities have a single local clock.
var Regions = []Stream{{Tag: "uk"}, {Tag: "europe"}, {Tag: "asia"}, {Tag: "mena"}, {Tag: "us"}}

type City struct{ Tag, Timezone string }

var Cities = []City{
	{"london", "Europe/London"}, {"paris", "Europe/Paris"},
	{"nyc", "America/New_York"}, {"sf", "America/Los_Angeles"},
	{"dubai", "Asia/Dubai"}, {"singapore", "Asia/Singapore"},
}

// RunSource keeps an explicitly selected agent stream current, without repeats.
// A zero source time means a shared stream has no single local time of day.
func RunSource(ctx context.Context, stream string, source Source, publish PublishPhoto) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if ctx.Err() != nil {
				return
			}
			post, err := source(ctx, time.Time{})
			if err == nil && post.Text != "" {
				err = publish(ctx, stream, post.Text, post.Name, post.Photo, time.Now().UTC().Format("2006-01-02T15"))
			}
			if err != nil && ctx.Err() == nil {
				log.Printf("%s: source unavailable", stream)
			}
			timer.Reset(10 * time.Minute)
		}
	}
}
