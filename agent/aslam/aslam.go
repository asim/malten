// Package aslam maintains sourced context for praise and gratitude.
package aslam

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/asim/malten/agent"
	"time"
)

func New() agent.Agent {
	return agent.Agent{
		Name:      "aslam",
		Objective: "Praise and gratitude. Preserve sourced prayers, English meanings, references and occasions from Aslam. Use real observations and natural conditions to recognise an appropriate moment for gratitude or remembrance. Acknowledge hardship without imposing positivity. Quote only complete supplied wording, never reconstruct truncated search excerpts, and keep any generated reflection distinct. Morning/evening prayers require matching local time evidence; otherwise use general remembrance. Do not post merely because another hour passed.",
		Read:      Read,
	}
}
func Read(ctx context.Context, _ time.Time) (json.RawMessage, error) {
	raw, err := agent.ReadJSON(ctx, "GET", "https://aslam.org/api/search?q=gratitude", "")
	if err != nil {
		return nil, err
	}
	var data struct{ Results []json.RawMessage }
	if json.Unmarshal(raw, &data) != nil || len(data.Results) == 0 {
		return nil, errors.New("empty Aslam source")
	}
	return json.Marshal(struct {
		Source string
		Data   json.RawMessage
	}{"https://aslam.org/api/search?q=gratitude", raw})
}
