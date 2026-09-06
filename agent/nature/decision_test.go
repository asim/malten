package nature

import (
	"context"
	"encoding/json"
	"github.com/asim/malten/agent"
	"strings"
	"testing"
	"time"
)

func TestInitialWeatherAndChanges(t *testing.T) {
	now := time.Now()
	p := weatherPlace{Place: "london", Name: "London", Timezone: "Europe/London"}
	p.Data.Current = conditions{Time: now.Unix(), Temperature: 20, Code: 2, Day: 1}
	p.Data.Daily.Sunrise = []int64{now.Add(-time.Hour).Unix()}
	p.Data.Daily.Sunset = []int64{now.Add(6 * time.Hour).Unix()}
	raw, _ := json.Marshal(struct{ Places []weatherPlace }{[]weatherPlace{p}})
	v := agent.View{Now: now, Records: []agent.Record{{ID: "source", Kind: "source", Data: raw, At: now}}}
	first, err := decide(context.Background(), v)
	if err != nil || first.Action == nil || !strings.Contains(first.Action.Text, "20°C") || !strings.Contains(first.Action.Text, "Weather estimate") {
		t.Fatalf("no initial weather: %+v %v", first, err)
	}
	v.Records = append(v.Records, agent.Record{ID: "sent", Kind: "decision", Action: first.Action, Summary: first.Summary, Status: "sent", At: now})
	unchanged, err := decide(context.Background(), v)
	if err != nil || unchanged.Action != nil {
		t.Fatal("unchanged weather reposted")
	}
	p.Data.Current.Code = 61
	raw, _ = json.Marshal(struct{ Places []weatherPlace }{[]weatherPlace{p}})
	v.Records = append(v.Records, agent.Record{ID: "rain", Kind: "source", Data: raw, At: now})
	changed, err := decide(context.Background(), v)
	if err != nil || changed.Action == nil || !strings.Contains(changed.Action.Text, "Rain") {
		t.Fatal("missed weather change")
	}
	v.SourceUnavailable = true
	stale, _ := decide(context.Background(), v)
	if stale.Action != nil {
		t.Fatal("reposted stale weather")
	}
}
