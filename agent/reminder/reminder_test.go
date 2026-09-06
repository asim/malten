package reminder

import (
	"context"
	"encoding/json"
	"github.com/asim/malten/agent"
	"strings"
	"testing"
	"time"
)

func TestSourcePassageDoesNotNeedModerationIncident(t *testing.T) {
	now := time.Now()
	v := agent.View{Now: now, Records: []agent.Record{{ID: "source", Kind: "source", At: now, Data: json.RawMessage(`{"Data":{"Verse":"Complete sourced passage","Hadith":"kept separately"}}`)}}}
	d, err := decide(context.Background(), v)
	if err != nil || d.Action == nil || !check(v, d) || !strings.HasPrefix(d.Action.Text, "Complete sourced passage") {
		t.Fatalf("empty source: %+v %v", d, err)
	}
	v.Records = append(v.Records, agent.Record{Status: "sent", At: now, Action: d.Action})
	d, err = decide(context.Background(), v)
	if err != nil || d.Action != nil {
		t.Fatal("repeated unchanged passage")
	}
}
