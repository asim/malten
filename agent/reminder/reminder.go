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
		Name:      "reminder",
		Objective: "Moderation engine. Preserve Quran, hadith, names of Allah and the separately generated reflection from reminder.dev. Maintain context for truthfulness, mercy, modesty and dignity under the fixed moderation policy. Publish a short general conduct guideline only when repeated confirmed moderation events in a stream warrant it. Never identify anyone, quote rejected content, infer faith, rewrite scripture, or change moderation rules. Otherwise update context without posting.",
		Read:      Read, Check: check,
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
