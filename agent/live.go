package agent

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"
)

// Post is a candidate from a source, or a recent post used to avoid repetition.
type Post struct {
	Text, Photo, Name string
	Created           int64
}
type Source func(context.Context, time.Time) (Post, error)
type zone struct {
	location *time.Location
	seen     time.Time
	checked  string
}

// Live gives the sources a shared hourly allowance in each quiet timezone stream.
type Live struct {
	sync.Mutex
	zones   map[string]zone
	sources []Source
	recent  func(string) []Post
	publish PublishPhoto
}

func NewLive(recent func(string) []Post, publish PublishPhoto, sources ...Source) *Live {
	return &Live{zones: map[string]zone{}, sources: sources, recent: recent, publish: publish}
}
func (l *Live) Observe(stream, name string) {
	if name == "" || name == "Local" || len(name) > 100 {
		return
	}
	if strings.NewReplacer("/", "-", "_", "-", "+", "-plus-").Replace(strings.ToLower(name)) != stream {
		return
	}
	l.Lock()
	defer l.Unlock()
	if z, ok := l.zones[stream]; ok {
		z.seen = time.Now()
		l.zones[stream] = z
		return
	}
	if len(l.zones) >= 256 {
		return
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return
	}
	l.zones[stream] = zone{location: loc, seen: time.Now()}
}
func (l *Live) Run(ctx context.Context) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			l.check(ctx, time.Now())
			timer.Reset(time.Minute)
		}
	}
}
func (l *Live) check(ctx context.Context, now time.Time) {
	l.Lock()
	zones := map[string]zone{}
	for stream, z := range l.zones {
		if now.Sub(z.seen) > 24*time.Hour {
			delete(l.zones, stream)
		} else {
			zones[stream] = z
		}
	}
	l.Unlock()
	for stream, z := range zones {
		if ctx.Err() != nil {
			return
		}
		local := now.In(z.location)
		key := local.Format("2006-01-02T15-0700")
		if z.checked == key || len(l.sources) == 0 {
			continue
		}
		recent := l.recent(stream)
		quiet := true
		for _, p := range recent {
			if now.Sub(time.UnixMilli(p.Created)) < time.Hour {
				quiet = false
				break
			}
		}
		if !quiet {
			continue
		}
		// One source-selection attempt per local hour, including failures.
		l.Lock()
		if current, ok := l.zones[stream]; ok {
			current.checked = key
			l.zones[stream] = current
		}
		l.Unlock()
		for i := 0; i < len(l.sources); i++ {
			if ctx.Err() != nil {
				return
			}
			post, err := l.sources[(local.Hour()+i)%len(l.sources)](ctx, local)
			if err != nil || strings.TrimSpace(post.Text) == "" {
				continue
			}
			repeated := false
			for _, p := range recent {
				if p.Text == post.Text {
					repeated = true
					break
				}
			}
			if repeated {
				continue
			}
			if err = l.publish(ctx, stream, post.Text, post.Name, post.Photo, key); err != nil {
				if ctx.Err() == nil {
					log.Print("agents: could not publish")
				}
				continue
			}
			break
		}
	}
}
