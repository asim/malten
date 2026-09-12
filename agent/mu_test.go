package agent_test

import (
	"context"
	"encoding/json"
	"github.com/asim/malten/agent"
	"github.com/asim/malten/agent/micro"
	"github.com/asim/malten/agent/nature"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestMuScopedSources(t *testing.T) {
	t.Setenv("MU_API_KEY", "fixture-secret")
	old := http.DefaultClient
	defer func() { http.DefaultClient = old }()
	calls := 0
	http.DefaultClient = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != "https://micro.mu/mcp" || r.Header.Get("Authorization") != "Bearer fixture-secret" {
			t.Fatal("wrong Mu authorization scope")
		}
		var request struct {
			Params struct {
				Name      string
				Arguments json.RawMessage
			}
		}
		json.NewDecoder(r.Body).Decode(&request)
		var text string
		switch request.Params.Name {
		case "news_list":
			text = `{"items":[{"title":"Headline","url":"https://example.com/story"}]}`
		case "news_search":
			text = `{"results":[{"title":"A relevant headline","url":"https://example.com/story","posted_at":"2026-09-12"}]}`
		case "places_geocode":
			text = "London — 51.5074, -0.1278"
		case "weather_forecast":
			text = "Source: weather provider. Current conditions 16°C; retrieved today."
		default:
			t.Fatal("unexpected tool", request.Params.Name)
		}
		raw, _ := json.Marshal(map[string]any{"result": map[string]any{"content": []any{map[string]string{"type": "text", "text": text}}}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
	})}
	if _, err := agent.MuCall(context.Background(), "mail_inbox", nil); err == nil || calls != 0 {
		t.Fatal("private tool reached Mu")
	}
	raw, err := micro.Read(context.Background(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(micro.Context(agent.Record{Data: raw, At: time.Now()})) != 1 {
		t.Fatal("news_list source lost")
	}
	sources, err := micro.Researcher().Search(context.Background(), "nature")
	if err != nil || len(sources) != 1 || !strings.Contains(sources[0].Text, "2026-09-12") {
		t.Fatal("dated news search lost", err)
	}
	lookups := nature.Researcher().Lookups
	if len(lookups) != 2 {
		t.Fatal("missing Mu nature tools")
	}
	if _, err := lookups[0].Read(context.Background(), json.RawMessage(`{"address":"London"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := lookups[1].Read(context.Background(), json.RawMessage(`{"lat":51.5074,"lon":-0.1278}`)); err != nil {
		t.Fatal(err)
	}
	before := calls
	if _, err := lookups[1].Read(context.Background(), json.RawMessage(`{"lat":91,"lon":0}`)); err == nil || calls != before {
		t.Fatal("invalid coordinates sent")
	}
}

func TestMuErrorsAndRedirectsDoNotLeakKey(t *testing.T) {
	t.Setenv("MU_API_KEY", "fixture-secret")
	old := http.DefaultClient
	defer func() { http.DefaultClient = old }()
	for _, status := range []int{302, 401, 200} {
		calls := 0
		http.DefaultClient = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: status, Header: http.Header{"Location": []string{"https://other.test/"}}, Body: io.NopCloser(strings.NewReader(`{"result":{"isError":true,"content":[{"type":"text","text":"fixture-secret"}]}}`))}, nil
		})}
		_, err := agent.MuCall(context.Background(), "news_list", map[string]any{})
		if err == nil || strings.Contains(err.Error(), "fixture-secret") || calls != 1 {
			t.Fatal("unsafe Mu failure", err)
		}
	}
}
