package agent_test

import (
	"context"
	"encoding/json"
	"github.com/asim/malten/agent"
	"github.com/asim/malten/agent/aslam"
	"github.com/asim/malten/agent/nature"
	"github.com/asim/malten/agent/news"
	"github.com/asim/malten/agent/reminder"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type transport func(*http.Request) (*http.Response, error)

func (t transport) RoundTrip(r *http.Request) (*http.Response, error) { return t(r) }
func TestSourcesPreserveDocuments(t *testing.T) {
	old := http.DefaultClient
	defer func() { http.DefaultClient = old }()
	fixtures := map[string]string{
		"reminder.dev": `{"verse":"complete verse","hadith":"complete hadith","name":"name and meaning","message":"generated reflection","links":{"verse":"/quran/1"},"updated":"source date"}`,
		"aslam.org":    `{"results":[{"Content":"Complete source wording","Title":"Reference","Role":"daily dua","Kind":"adhkar","URL":"/adhkar/reference"}]}`,
		"micro.mu":     `{"result":{"content":[{"type":"text","text":"{\"items\":[{\"title\":\"One\",\"url\":\"https://example.com\",\"category\":\"world\"}]}"}]}}`,
	}
	http.DefaultClient = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(fixtures[r.URL.Host]))}, nil
	})}
	for _, worker := range []agent.Agent{reminder.New(), aslam.New(), news.New()} {
		raw, err := worker.Read(context.Background(), time.Now())
		if err != nil {
			t.Fatalf("%s: %v", worker.Name, err)
		}
		var wrapper struct{ Data json.RawMessage }
		if json.Unmarshal(raw, &wrapper) != nil {
			t.Fatal("invalid record")
		}
		var host string
		switch worker.Name {
		case "reminder":
			host = "reminder.dev"
		case "aslam":
			host = "aslam.org"
		case "news":
			host = "micro.mu"
		}
		var want, got any
		json.Unmarshal([]byte(fixtures[host]), &want)
		json.Unmarshal(wrapper.Data, &got)
		a, _ := json.Marshal(want)
		b, _ := json.Marshal(got)
		if string(a) != string(b) {
			t.Fatalf("%s discarded source fields", worker.Name)
		}
	}
	fixtures["micro.mu"] = `{"result":{"isError":true}}`
	if _, err := news.Read(context.Background(), time.Now()); err == nil {
		t.Fatal("accepted news error")
	}
	fixtures["reminder.dev"] = `{"message":"only a reflection"}`
	if _, err := reminder.Read(context.Background(), time.Now()); err == nil {
		t.Fatal("accepted incomplete reminder")
	}
}
func TestNatureWeatherSourceAndFreshness(t *testing.T) {
	now := time.Now()
	old := http.DefaultClient
	defer func() { http.DefaultClient = old }()
	t.Setenv("OPEN_METEO_API_KEY", "")
	stale := false
	http.DefaultClient = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.open-meteo.com" || r.URL.Query().Get("daily") != "sunrise,sunset" || r.URL.Query().Get("current") == "" {
			t.Fatalf("wrong request: %s", r.URL.Host)
		}
		var rows []any
		at := now
		if stale {
			at = now.Add(-2 * time.Hour)
		}
		for _, city := range agent.Cities {
			rows = append(rows, map[string]any{"timezone": city.Timezone, "current": map[string]any{"time": at.Unix(), "temperature_2m": 21, "weather_code": 1}, "daily": map[string]any{"sunrise": []int64{now.Add(-time.Hour).Unix()}, "sunset": []int64{now.Add(8 * time.Hour).Unix()}}})
		}
		raw, _ := json.Marshal(rows)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
	})}
	raw, err := nature.Read(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "San Francisco") {
		t.Fatal("missing place context")
	}
	photo, caption := nature.New().Media(agent.Action{Place: "london"})
	if !strings.HasPrefix(photo, "data:image/jpeg;base64,") || !strings.Contains(caption, "not live") {
		t.Fatal("missing attributed illustrative image")
	}
	stale = true
	if _, err = nature.Read(context.Background(), now); err == nil {
		t.Fatal("accepted stale current weather")
	}
}
func TestReminderGuidelinesNeedRepeatedIncidents(t *testing.T) {
	now := time.Now()
	v := agent.View{Now: now}
	d := agent.Decision{Action: &agent.Action{Stream: "home", Text: "Treat people with dignity."}}
	r := reminder.New()
	if r.Check(v, d) {
		t.Fatal("unsolicited guidelines")
	}
	for i := 0; i < 3; i++ {
		v.Records = append(v.Records, agent.Record{Kind: "moderation", At: now, Data: json.RawMessage(`{"Stream":"home"}`)})
	}
	if !r.Check(v, d) {
		t.Fatal("ignored repeated incidents")
	}
	v.Records = append(v.Records, agent.Record{Action: d.Action, At: now})
	if r.Check(v, d) {
		t.Fatal("repeated guideline")
	}
}
