// Package llm is a tiny, dependency-free client for Anthropic's Messages API.
// It speaks HTTP + SSE directly so Malten keeps its no-external-Go-dependency
// invariant (see CLAUDE.md): the whole binary is still self-contained.
//
// It implements just enough to run a bounded tool-use loop and stream the
// assistant's text back token by token — which is all "Ask Malten" needs.
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	apiURL     = "https://api.anthropic.com/v1/messages"
	apiVersion = "2023-06-01"
)

// Tool is a function the model can call during a turn.
type Tool struct {
	Name        string
	Description string
	// Schema is the JSON Schema for the tool input (an "object" schema).
	Schema map[string]any
	// Run executes the tool. It receives the model-supplied input and returns
	// a string result that is fed back to the model.
	Run func(ctx context.Context, input json.RawMessage) (string, error)
}

// Client talks to the Messages API.
type Client struct {
	APIKey    string
	Model     string
	MaxTokens int
	URL       string // defaults to the Anthropic Messages API; overridable for tests
	HTTP      *http.Client
}

// New builds a Client. Model defaults to claude-opus-4-8.
func New(apiKey, model string) *Client {
	if model == "" {
		model = "claude-opus-4-8"
	}
	return &Client{
		APIKey:    apiKey,
		Model:     model,
		MaxTokens: 1024,
		URL:       apiURL,
		HTTP:      &http.Client{Timeout: 90 * time.Second},
	}
}

// Event is emitted as a turn progresses.
type Event struct {
	Type string // "text" | "tool" | "error"
	Text string // token text (Type=="text") or message (Type=="error")
	Tool string // tool name (Type=="tool")
}

// --- wire types (only the fields we use) ------------------------------------

type contentBlock struct {
	Type string `json:"type"`
	// text
	Text string `json:"text,omitempty"`
	// tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
	// tool_result
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

type message struct {
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`
}

type toolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type request struct {
	Model     string     `json:"model"`
	MaxTokens int        `json:"max_tokens"`
	System    string     `json:"system,omitempty"`
	Messages  []message  `json:"messages"`
	Tools     []toolSpec `json:"tools,omitempty"`
	Stream    bool       `json:"stream"`
}

// Turn is one message in an exchange. A conversation is replayed as turns so
// the model can answer follow-ups ("is it open?", "and the one after that?").
type Turn struct {
	Role string // "user" | "assistant"
	Text string
}

// Run executes a bounded agent loop for a single question. It streams assistant
// text to emit, runs any tools the model calls, and continues until the model
// stops asking for tools (or a safety cap is hit).
func (c *Client) Run(ctx context.Context, system, userMsg string, tools []Tool, emit func(Event)) error {
	return c.RunTurns(ctx, system, []Turn{{Role: "user", Text: userMsg}}, tools, emit)
}

// RunTurns is Run over a conversation. The turns are normalised first — the
// Messages API requires a user turn to start and roles to alternate, and the
// history comes from a browser, so it can't be trusted to be either.
func (c *Client) RunTurns(ctx context.Context, system string, turns []Turn, tools []Tool, emit func(Event)) error {
	byName := map[string]Tool{}
	specs := make([]toolSpec, 0, len(tools))
	for _, t := range tools {
		byName[t.Name] = t
		specs = append(specs, toolSpec{Name: t.Name, Description: t.Description, InputSchema: t.Schema})
	}

	msgs := normalise(turns)
	if len(msgs) == 0 {
		return fmt.Errorf("no message to send")
	}

	const maxTurns = 6
	for turn := 0; turn < maxTurns; turn++ {
		assistant, stop, err := c.stream(ctx, system, msgs, specs, emit)
		if err != nil {
			return err
		}
		msgs = append(msgs, message{Role: "assistant", Content: assistant})
		if stop != "tool_use" {
			return nil
		}

		// Run each requested tool and gather results into one user turn.
		var results []contentBlock
		for _, b := range assistant {
			if b.Type != "tool_use" {
				continue
			}
			emit(Event{Type: "tool", Tool: b.Name})
			res := contentBlock{Type: "tool_result", ToolUseID: b.ID}
			t, ok := byName[b.Name]
			if !ok {
				res.Content, res.IsError = "unknown tool", true
			} else if out, terr := t.Run(ctx, b.Input); terr != nil {
				res.Content, res.IsError = terr.Error(), true
			} else {
				res.Content = out
			}
			results = append(results, res)
		}
		msgs = append(msgs, message{Role: "user", Content: results})
	}
	return fmt.Errorf("stopped after %d turns", maxTurns)
}

// normalise turns a loose conversation into a valid message list: empty turns
// dropped, any leading assistant turns dropped, consecutive same-role turns
// merged, and anything that isn't "assistant" treated as a user turn.
func normalise(turns []Turn) []message {
	var msgs []message
	for _, t := range turns {
		text := strings.TrimSpace(t.Text)
		if text == "" {
			continue
		}
		role := "user"
		if t.Role == "assistant" {
			role = "assistant"
		}
		if len(msgs) == 0 && role == "assistant" {
			continue // a conversation must open with the user
		}
		if n := len(msgs); n > 0 && msgs[n-1].Role == role {
			msgs[n-1].Content[0].Text += "\n\n" + text
			continue
		}
		msgs = append(msgs, message{Role: role, Content: []contentBlock{{Type: "text", Text: text}}})
	}
	// A trailing assistant turn would leave the model nothing to answer.
	if n := len(msgs); n > 0 && msgs[n-1].Role == "assistant" {
		msgs = msgs[:n-1]
	}
	return msgs
}

// stream performs one streaming request, emitting text tokens as they arrive.
// It returns the assistant's assembled content blocks and the stop reason.
func (c *Client) stream(ctx context.Context, system string, msgs []message, tools []toolSpec, emit func(Event)) ([]contentBlock, string, error) {
	body, err := json.Marshal(request{
		Model:     c.Model,
		MaxTokens: c.MaxTokens,
		System:    system,
		Messages:  msgs,
		Tools:     tools,
		Stream:    true,
	})
	if err != nil {
		return nil, "", err
	}
	endpoint := c.URL
	if endpoint == "" {
		endpoint = apiURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("anthropic-version", apiVersion)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("anthropic status %d", resp.StatusCode)
	}

	// Accumulate content blocks by index. tool_use inputs arrive as a stream of
	// partial-JSON deltas we concatenate, then parse at content_block_stop.
	blocks := map[int]*contentBlock{}
	partial := map[int]*strings.Builder{}
	stopReason := ""

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		var ev struct {
			Type         string `json:"type"`
			Index        int    `json:"index"`
			ContentBlock *struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"content_block"`
			Delta *struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				PartialJSON string `json:"partial_json"`
				StopReason  string `json:"stop_reason"`
			} `json:"delta"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			continue
		}
		switch ev.Type {
		case "content_block_start":
			b := &contentBlock{Type: "text"}
			if ev.ContentBlock != nil {
				b.Type = ev.ContentBlock.Type
				b.ID = ev.ContentBlock.ID
				b.Name = ev.ContentBlock.Name
			}
			blocks[ev.Index] = b
			partial[ev.Index] = &strings.Builder{}
		case "content_block_delta":
			if ev.Delta == nil {
				break
			}
			switch ev.Delta.Type {
			case "text_delta":
				if b := blocks[ev.Index]; b != nil {
					b.Text += ev.Delta.Text
				}
				if ev.Delta.Text != "" {
					emit(Event{Type: "text", Text: ev.Delta.Text})
				}
			case "input_json_delta":
				if p := partial[ev.Index]; p != nil {
					p.WriteString(ev.Delta.PartialJSON)
				}
			}
		case "content_block_stop":
			if b := blocks[ev.Index]; b != nil && b.Type == "tool_use" {
				if p := partial[ev.Index]; p != nil && p.Len() > 0 {
					b.Input = json.RawMessage(p.String())
				} else {
					b.Input = json.RawMessage("{}")
				}
			}
		case "message_delta":
			if ev.Delta != nil && ev.Delta.StopReason != "" {
				stopReason = ev.Delta.StopReason
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, "", err
	}

	// Emit blocks in index order.
	out := make([]contentBlock, 0, len(blocks))
	for i := 0; i < len(blocks); i++ {
		if b := blocks[i]; b != nil {
			out = append(out, *b)
		}
	}
	return out, stopReason, nil
}
