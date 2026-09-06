package daylight

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/asim/malten/agent"
	"image/jpeg"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCityDateCacheAndPhotos(t *testing.T) {
	old := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = old })
	calls := 0
	http.DefaultClient = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		calls++
		q := r.URL.Query()
		loc, err := time.LoadLocation(q.Get("tzid"))
		if err != nil {
			t.Fatal(err)
		}
		date, err := time.ParseInLocation("2006-01-02", q.Get("date"), loc)
		if err != nil {
			t.Fatal(err)
		}
		if q.Get("lat") == "" || q.Get("lng") == "" || q.Get("formatted") != "0" {
			t.Fatal("missing solar request parameters")
		}
		payload, _ := json.Marshal(map[string]any{"status": "OK", "results": map[string]time.Time{"sunrise": date.Add(7 * time.Hour).UTC(), "sunset": date.Add(19 * time.Hour).UTC()}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(payload))}, nil
	})}
	a := New()
	for _, city := range agent.Cities {
		loc, _ := time.LoadLocation(city.Timezone)
		local := time.Date(2026, 9, 7, 0, 30, 0, 0, loc)
		post, err := a.Fetch(context.Background(), local)
		if err != nil {
			t.Fatal(city.Tag, err)
		}
		if !strings.Contains(post.Text, "Before sunrise") || !strings.Contains(post.Text, "Sunrise 07:00") || !strings.Contains(post.Text, "not a live view") || !strings.Contains(post.Text, "https://sunrise-sunset.org") {
			t.Fatal(post.Text)
		}
		raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(post.Photo, "data:image/jpeg;base64,"))
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(raw))
		if err != nil || cfg.Width > 1280 || cfg.Height > 1280 || len(raw) > 400*1024 {
			t.Fatal("invalid bundled photo", city.Tag)
		}
		before := calls
		post, err = a.Fetch(context.Background(), local.Add(12*time.Hour))
		if err != nil || calls != before || !strings.Contains(post.Text, "Daylight.") {
			t.Fatal("day cache or phase", post.Text, err)
		}
		_, err = a.Fetch(context.Background(), local.AddDate(0, 0, 1))
		if err != nil || calls != before+1 {
			t.Fatal("did not request next local date")
		}
	}
	before := calls
	post, err := a.Fetch(context.Background(), time.Time{})
	if err != nil || post.Text != "" || calls != before {
		t.Fatal("broad region assigned a sunrise")
	}
}

func TestRejectInvalidAndStaleSolarTimes(t *testing.T) {
	local := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	for _, data := range []string{
		`{"status":"UNKNOWN_ERROR"}`, `{"status":"OK","results":{}}`,
		`{"status":"OK","results":{"sunrise":"2026-09-06T07:00:00Z","sunset":"2026-09-06T19:00:00Z"}}`,
		`{"status":"OK","results":{"sunrise":"2026-09-07T19:00:00Z","sunset":"2026-09-07T07:00:00Z"}}`,
	} {
		if _, err := readDay(strings.NewReader(data), local); err == nil {
			t.Fatal("accepted invalid day", data)
		}
	}
	loc, _ := time.LoadLocation("Europe/London")
	day, err := readDay(strings.NewReader(`{"status":"OK","results":{"sunrise":"2026-09-07T05:30:00Z","sunset":"2026-09-07T18:30:00Z"}}`), local.In(loc))
	if err != nil || day.Sunrise.Hour() != 6 || day.Sunset.Hour() != 19 {
		t.Fatal("DST conversion", day, err)
	}
}

func TestDaylightStopsWithServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		New().Run(ctx, func(context.Context, string, string, string, string, ...string) error {
			t.Error("published after shutdown")
			return nil
		})
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("did not stop")
	}
}
