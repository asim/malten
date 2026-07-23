package llm

import (
	"context"
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
)

// Claude is the real backend, backed by the Anthropic Messages API. It maps our
// provider-agnostic types to the SDK's and back.
//
// Thinking is left off: customer support is not a high-reasoning task, so we
// favour lower latency and cost. Adjust the model via the constructor.
type Claude struct {
	client anthropic.Client
	model  anthropic.Model
	maxTok int64
}

// NewClaude builds a Claude backend. The API key is read from ANTHROPIC_API_KEY
// (or an `ant auth login` profile) by the SDK. model may be empty to default to
// Claude Opus 4.8.
func NewClaude(model string) *Claude {
	m := anthropic.ModelClaudeOpus4_8
	if model != "" {
		m = anthropic.Model(model)
	}
	return &Claude{
		client: anthropic.NewClient(),
		model:  m,
		maxTok: 1024,
	}
}

// Name identifies the backend and model.
func (c *Claude) Name() string { return "claude:" + string(c.model) }

// Complete runs one model turn.
func (c *Claude) Complete(ctx context.Context, req Request) (*Response, error) {
	params := anthropic.MessageNewParams{
		Model:     c.model,
		MaxTokens: c.maxTok,
		Messages:  toParams(req.Messages),
		Tools:     toToolUnions(req.Tools),
	}
	if req.System != "" {
		params.System = []anthropic.TextBlockParam{{Text: req.System}}
	}

	msg, err := c.client.Messages.New(ctx, params)
	if err != nil {
		return nil, err
	}

	out := &Response{}
	switch msg.StopReason {
	case anthropic.StopReasonToolUse:
		out.StopReason = StopToolUse
	default:
		out.StopReason = StopEndTurn
	}
	for _, block := range msg.Content {
		switch v := block.AsAny().(type) {
		case anthropic.TextBlock:
			out.Content = append(out.Content, Text(v.Text))
		case anthropic.ToolUseBlock:
			out.Content = append(out.Content, ToolUse(v.ID, v.Name, json.RawMessage(v.JSON.Input.Raw())))
		}
	}
	return out, nil
}

// toParams converts our transcript to SDK message params, preserving tool_use
// and tool_result blocks so multi-step context replays correctly.
func toParams(msgs []Message) []anthropic.MessageParam {
	var out []anthropic.MessageParam
	for _, m := range msgs {
		var blocks []anthropic.ContentBlockParamUnion
		for _, b := range m.Content {
			switch b.Type {
			case BlockText:
				blocks = append(blocks, anthropic.NewTextBlock(b.Text))
			case BlockToolUse:
				blocks = append(blocks, anthropic.ContentBlockParamUnion{
					OfToolUse: &anthropic.ToolUseBlockParam{
						ID:    b.ID,
						Name:  b.Name,
						Input: json.RawMessage(b.Input),
					},
				})
			case BlockToolResult:
				blocks = append(blocks, anthropic.NewToolResultBlock(b.ToolUseID, b.Content, b.IsError))
			}
		}
		if m.Role == RoleUser {
			out = append(out, anthropic.NewUserMessage(blocks...))
		} else {
			out = append(out, anthropic.NewAssistantMessage(blocks...))
		}
	}
	return out
}

func toToolUnions(defs []ToolDef) []anthropic.ToolUnionParam {
	var out []anthropic.ToolUnionParam
	for _, d := range defs {
		t := anthropic.ToolParam{
			Name:        d.Name,
			Description: anthropic.String(d.Description),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: d.Properties,
				Required:   d.Required,
			},
		}
		out = append(out, anthropic.ToolUnionParam{OfTool: &t})
	}
	return out
}
