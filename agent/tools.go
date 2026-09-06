package agent

import (
	"context"
	"encoding/json"
)

// Tool is a read-only capability offered to the model for the current task.
type Tool struct {
	Name        string                                                          `json:"name"`
	Description string                                                          `json:"description"`
	InputSchema any                                                             `json:"input_schema"`
	Call        func(context.Context, json.RawMessage) (json.RawMessage, error) `json:"-"`
}

// Source keeps retrieved wording separate from generated interpretation.
type Source struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Text    string `json:"text,omitempty"`
	Excerpt bool   `json:"excerpt,omitempty"`
}

func NewSource(title, address, text string, excerpt bool) Source {
	return Source{ID: Key([]byte(address + "\n" + text))[:16], Title: title, URL: address, Text: text, Excerpt: excerpt}
}
