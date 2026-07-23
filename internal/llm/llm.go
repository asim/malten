// Package llm defines a provider-agnostic interface for the language model
// that drives the Malten support agent, plus the small set of message and
// tool types the agent loop is built on.
//
// The interface is deliberately narrow: a single Complete call that takes the
// conversation so far plus the available tools and returns either final text
// or a set of tool calls. Any backend (the real Claude API, or the built-in
// deterministic Stub) can satisfy it, which is what makes the agent loop
// testable without spending money or requiring network access.
package llm

import (
	"context"
	"encoding/json"
)

// Role is the author of a message in a conversation.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// BlockType enumerates the kinds of content block a message can carry. This
// mirrors the Anthropic Messages API content model so the Claude backend maps
// across cleanly, but it is intentionally minimal.
type BlockType string

const (
	BlockText       BlockType = "text"
	BlockToolUse    BlockType = "tool_use"
	BlockToolResult BlockType = "tool_result"
)

// Block is a single piece of message content. Only the fields relevant to the
// Type are populated; the JSON tags keep persisted history compact.
type Block struct {
	Type BlockType `json:"type"`

	// Text content (Type == BlockText).
	Text string `json:"text,omitempty"`

	// Tool call the model wants to make (Type == BlockToolUse).
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// Result of executing a tool call (Type == BlockToolResult).
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

// Text builds a text block.
func Text(s string) Block { return Block{Type: BlockText, Text: s} }

// ToolUse builds a tool_use block.
func ToolUse(id, name string, input json.RawMessage) Block {
	return Block{Type: BlockToolUse, ID: id, Name: name, Input: input}
}

// ToolResult builds a tool_result block.
func ToolResult(toolUseID, content string, isErr bool) Block {
	return Block{Type: BlockToolResult, ToolUseID: toolUseID, Content: content, IsError: isErr}
}

// Message is one turn in the conversation. Consecutive same-role messages are
// allowed (the agent uses a user message of tool_result blocks to feed results
// back to the model).
type Message struct {
	Role    Role    `json:"role"`
	Content []Block `json:"content"`
}

// UserText is a convenience constructor for a plain user message.
func UserText(s string) Message {
	return Message{Role: RoleUser, Content: []Block{Text(s)}}
}

// ToolDef describes a tool the model may call. Properties/Required form a JSON
// Schema object; keeping them split makes it trivial to hand to the Claude SDK.
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Properties  map[string]any `json:"properties"`
	Required    []string       `json:"required"`
}

// JSONSchema renders the tool's input schema as a standard JSON Schema object.
func (d ToolDef) JSONSchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": d.Properties,
		"required":   d.Required,
	}
}

// Stop reasons returned by a backend.
const (
	StopEndTurn = "end_turn"
	StopToolUse = "tool_use"
)

// Request is a single completion request.
type Request struct {
	System   string
	Messages []Message
	Tools    []ToolDef
}

// Response is what a backend returns for a Request.
type Response struct {
	StopReason string
	Content    []Block
}

// ToolUses returns the tool_use blocks in the response, if any.
func (r *Response) ToolUses() []Block {
	var out []Block
	for _, b := range r.Content {
		if b.Type == BlockToolUse {
			out = append(out, b)
		}
	}
	return out
}

// Text concatenates the text blocks in the response.
func (r *Response) Text() string {
	var s string
	for _, b := range r.Content {
		if b.Type == BlockText {
			s += b.Text
		}
	}
	return s
}

// LLM is the provider-agnostic model interface the agent depends on.
type LLM interface {
	// Name identifies the backing implementation (e.g. "stub", "claude:...").
	Name() string
	// Complete runs one model turn.
	Complete(ctx context.Context, req Request) (*Response, error)
}
