// Package news maintains a current, attributed view of headline changes.
package news

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/asim/malten/agent"
	"time"
)

func New() agent.Agent {
	return agent.Agent{
		Name:      "news",
		Objective: "Brief generation. Track new and changing stories against earlier headlines and summaries. Maintain a concise view of now, preserving sources and uncertainty. Post a short headline-based brief in news only when there is a meaningful development. Headlines alone are not full reporting: do not invent context, causal explanations or details. Cite original article links. Avoid sensationalism and repeated briefs about unchanged stories.",
		Read:      Read,
	}
}
func Read(ctx context.Context, _ time.Time) (json.RawMessage, error) {
	raw, err := agent.ReadJSON(ctx, "POST", "https://micro.mu/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"news_headlines","arguments":{}}}`)
	if err != nil {
		return nil, err
	}
	var response struct {
		Error  json.RawMessage
		Result struct {
			IsError bool
			Content []struct{ Type, Text string }
		}
	}
	if json.Unmarshal(raw, &response) != nil || (len(response.Error) > 0 && string(response.Error) != "null") || response.Result.IsError {
		return nil, errors.New("news tool failed")
	}
	usable := false
	for _, c := range response.Result.Content {
		var headlines struct{ Items []json.RawMessage }
		if c.Type == "text" && json.Unmarshal([]byte(c.Text), &headlines) == nil && len(headlines.Items) > 0 {
			usable = true
		}
	}
	if !usable {
		return nil, errors.New("empty headlines")
	}
	return json.Marshal(struct {
		Source string
		Data   json.RawMessage
	}{"https://micro.mu/mcp — news_headlines", raw})
}
