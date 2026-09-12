package news

import (
	"context"
	"encoding/json"
	"github.com/asim/malten/agent"
	"time"
)

func Researcher() agent.Researcher {
	r := agent.Researcher{Name: "news", Objective: "Establish what the available headlines say about the event in the question. Read retained context or refresh the headlines, then select relevant stories. Headlines alone are not full reporting: do not infer causes, article details or confirmation of a person's account. Preserve article links, freshness and uncertainty. If the event is absent, say it is not established by these sources.", Read: Read, Sources: Context}
	if agent.MuAvailable() {
		r.Search = search
	}
	return r
}
func search(ctx context.Context, query string) ([]agent.Source, error) {
	text, err := agent.MuCall(ctx, "news_search", map[string]string{"query": query})
	if err != nil {
		return nil, err
	}
	var found struct{ Results []json.RawMessage }
	if err = json.Unmarshal([]byte(text), &found); err != nil {
		return nil, err
	}
	items, _ := json.Marshal(map[string]any{"items": found.Results})
	raw, _ := json.Marshal(map[string]any{"Data": map[string]any{"Result": map[string]any{"Content": []any{map[string]string{"Type": "text", "Text": string(items)}}}}})
	return Context(agent.Record{Data: raw, At: time.Now()}), nil
}
