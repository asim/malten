package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"
)

type newsZone struct {
	location *time.Location
	seen     time.Time
	posted   string
}

// News publishes one morning post per active timezone, through normal moderation.
type News struct {
	sync.Mutex
	zones     map[string]newsZone
	publish   Publish
	headlines func(context.Context) (string, error)
}

func NewNews(publish Publish) *News {
	return &News{zones: map[string]newsZone{}, publish: publish, headlines: newsHeadlines}
}

// Observe accepts only a timezone matching the stream's normalised identifier.
func (n *News) Observe(stream, zone string) {
	if len(zone) > 100 || zone == "" || zone == "Local" {
		return
	}
	tag := strings.NewReplacer("/", "-", "_", "-", "+", "-plus-").Replace(strings.ToLower(zone))
	if stream != tag {
		return
	}
	n.Lock()
	defer n.Unlock()
	if existing, ok := n.zones[stream]; ok {
		existing.seen = time.Now()
		n.zones[stream] = existing
		return
	}
	if len(n.zones) >= 256 {
		return
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return
	}
	n.zones[stream] = newsZone{location: location, seen: time.Now()}
}

func (n *News) Run(ctx context.Context) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			n.check(ctx, time.Now())
			timer.Reset(time.Minute)
		}
	}
}

func (n *News) check(ctx context.Context, now time.Time) {
	pending := map[string]string{}
	n.Lock()
	for stream, zone := range n.zones {
		if now.Sub(zone.seen) > 24*time.Hour {
			delete(n.zones, stream)
			continue
		}
		local := now.In(zone.location)
		day := local.Format("2006-01-02")
		if local.Hour() == 8 && zone.posted != day {
			pending[stream] = day
		}
	}
	n.Unlock()
	if len(pending) == 0 || ctx.Err() != nil {
		return
	}
	text, err := n.headlines(ctx)
	if err != nil {
		if ctx.Err() == nil {
			log.Print("news: headlines unavailable")
		}
		return
	}
	for stream, day := range pending {
		if ctx.Err() != nil {
			return
		}
		if err := n.publish(ctx, stream, text, "News · Micro", day); err != nil {
			if ctx.Err() == nil {
				log.Print("news: could not publish morning headlines")
			}
			continue
		}
		n.Lock()
		if zone, ok := n.zones[stream]; ok {
			zone.posted = day
			n.zones[stream] = zone
		}
		n.Unlock()
	}
}

func newsHeadlines(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", "https://micro.mu/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"news_headlines","arguments":{}}}`))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", errors.New("headlines request failed")
	}
	return readHeadlines(io.LimitReader(res.Body, 256*1024))
}

func readHeadlines(reader io.Reader) (string, error) {
	var response struct {
		Error  json.RawMessage
		Result struct {
			IsError bool
			Content []struct{ Type, Text string }
		}
	}
	if err := json.NewDecoder(reader).Decode(&response); err != nil {
		return "", err
	}
	if (len(response.Error) > 0 && string(response.Error) != "null") || response.Result.IsError {
		return "", errors.New("headlines tool failed")
	}
	text := "Morning headlines"
	seen := map[string]bool{}
	count := 0
	for _, content := range response.Result.Content {
		if content.Type != "text" {
			continue
		}
		var headlines struct {
			Items []struct{ Category, Title, URL string }
		}
		if err := json.Unmarshal([]byte(content.Text), &headlines); err != nil {
			return "", err
		}
		for _, item := range headlines.Items {
			category, title := strings.TrimSpace(item.Category), strings.TrimSpace(item.Title)
			link, err := url.Parse(item.URL)
			if err != nil || link.Host == "" || (link.Scheme != "https" && link.Scheme != "http") || link.User != nil {
				continue
			}
			if category == "" || title == "" || seen[strings.ToLower(category)] {
				continue
			}
			entry := "\n\n" + category + " · " + title + "\n" + link.String()
			if len([]rune(text+entry)) > 1200 {
				continue
			}
			text += entry
			seen[strings.ToLower(category)] = true
			count++
			if count == 3 {
				return text, nil
			}
		}
	}
	if count == 0 {
		return "", errors.New("no usable headlines")
	}
	return text, nil
}
