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
		Messages:  toParams(balanceToolResults(req.Messages)),
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

// balanceToolResults enforces the Messages API contract that every tool_use is
// answered by a tool_result in the next message. Historical transcripts written
// before the escalation-persistence fix can contain orphaned tool_use blocks;
// rather than let the whole session fail with a 400, synthesize placeholder
// results (merging into the following user turn, or inserting one) so the
// conversation still replays.
func balanceToolResults(msgs []Message) []Message {
	out := make([]Message, 0, len(msgs)+2)
	for i := 0; i < len(msgs); i++ {
		m := msgs[i]
		out = append(out, m)

		var ids []string
		for _, b := range m.Content {
			if b.Type == BlockToolUse {
				ids = append(ids, b.ID)
			}
		}
		if len(ids) == 0 {
			continue
		}

		answered := map[string]bool{}
		nextIsResults := i+1 < len(msgs) && msgs[i+1].Role == RoleUser
		if nextIsResults {
			for _, b := range msgs[i+1].Content {
				if b.Type == BlockToolResult {
					answered[b.ToolUseID] = true
				}
			}
		}

		var missing []Block
		for _, id := range ids {
			if !answered[id] {
				missing = append(missing, ToolResult(id, "(no result recorded)", false))
			}
		}
		if len(missing) == 0 {
			continue
		}
		if nextIsResults {
			// Merge the synthesized results into the existing next user turn.
			merged := msgs[i+1]
			merged.Content = append(append([]Block{}, missing...), merged.Content...)
			out = append(out, merged)
			i++ // consumed msgs[i+1]
		} else {
			// The next message is not a user turn (or there is none): insert one.
			out = append(out, Message{Role: RoleUser, Content: missing})
		}
	}
	return out
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
