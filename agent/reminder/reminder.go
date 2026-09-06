// Package reminder maintains the spiritual source context supporting conduct.
package reminder

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/asim/malten/agent"
	"time"
)

func New() agent.Agent {
	return agent.Agent{
		DisplayName: "Reminder",
		Name:        "reminder",
		Objective:   "Moderation engine. Preserve Quran, hadith, names of Allah and the separately generated reflection from reminder.dev. Maintain context for truthfulness, mercy, modesty and dignity under the fixed moderation policy. Publish a short general conduct guideline only when repeated confirmed moderation events in a stream warrant it. Never identify anyone, quote rejected content, infer faith, rewrite scripture, or change moderation rules. Its own source stream can show a complete sourced passage. Otherwise update context without posting.",
		Read:        Read, Decide: decide, Check: check,
	}
}
func Read(ctx context.Context, _ time.Time) (json.RawMessage, error) {
	raw, err := agent.ReadJSON(ctx, "GET", "https://reminder.dev/api/latest", "")
	if err != nil {
		return nil, err
	}
	var data struct{ Verse, Hadith, Name, Message string }
	if json.Unmarshal(raw, &data) != nil || data.Verse == "" || data.Hadith == "" || data.Name == "" {
		return nil, errors.New("incomplete reminder source")
	}
	return json.Marshal(struct {
		Source string
		Data   json.RawMessage
	}{"https://reminder.dev/api/latest", raw})
}
func check(v agent.View, d agent.Decision) bool {
	if d.Action.Stream == "reminder" {
		for i := len(v.Records) - 1; i >= 0; i-- {
			r := v.Records[i]
			if r.Kind == "source" {
				return d.Action.Text == passage(r.Data)
			}
		}
		return false
	}
	// Individual rejections never prompt a public admonishment. Only anonymised
	// repeated confirmed incidents can warrant a general guideline.
	count := 0
	for _, r := range v.Records {
		if r.Kind == "moderation" && v.Now.Sub(r.At) < time.Hour {
			var event struct{ Stream string }
			if json.Unmarshal(r.Data, &event) == nil && event.Stream == d.Action.Stream {
				count++
			}
		}
		if r.Action != nil && r.Action.Stream == d.Action.Stream {
			return false
		}
	}
	return count >= 3
}

func passage(raw json.RawMessage) string {
	var source struct{ Data struct{ Verse string } }
	if json.Unmarshal(raw, &source) != nil || source.Data.Verse == "" {
		return ""
	}
	text := source.Data.Verse + "\n\nhttps://reminder.dev"
	if len([]rune(text)) > 1200 {
		return ""
	}
	return text
}
func decide(ctx context.Context, v agent.View) (agent.Decision, error) {
	// Conduct guidance is reactive; the source stream can still show a passage.
	counts := map[string]int{}
	for _, r := range v.Records {
		if r.Kind == "moderation" && v.Now.Sub(r.At) < time.Hour {
			var e struct{ Stream string }
			if json.Unmarshal(r.Data, &e) == nil {
				counts[e.Stream]++
			}
		}
	}
	for _, count := range counts {
		if count >= 3 {
			return agent.Decide(ctx, v)
		}
	}
	for i := len(v.Records) - 1; i >= 0; i-- {
		r := v.Records[i]
		if r.Kind != "source" {
			continue
		}
		text := passage(r.Data)
		d := agent.Decision{Summary: "Retained complete religious source material under the fixed moderation policy."}
		if text == "" || v.SourceUnavailable {
			return d, nil
		}
		for _, prior := range v.Records {
			if prior.Status == "sent" && prior.Action != nil && prior.Action.Stream == "reminder" && !prior.At.Before(r.At) {
				return d, nil
			}
		}
		d.Action = &agent.Action{Stream: "reminder", Text: text}
		d.Evidence = []string{r.ID}
		return d, nil
	}
	return agent.Decision{}, nil
}
