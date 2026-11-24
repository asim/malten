package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPStream(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s := New()
	s.Commands = make(chan *Message, 1)
	web := httptest.NewServer(MCPHandler(s))
	defer web.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: web.URL, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	tools, err := session.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 4 {
		t.Fatalf("list tools: %v, %v", tools, err)
	}
	call := func(name string, args any, out any) {
		t.Helper()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil || result.IsError {
			t.Fatalf("%s: %v, %+v", name, err, result)
		}
		if out != nil {
			if err := json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), out); err != nil {
				t.Fatal(err)
			}
		}
	}
	call("create_stream", map[string]any{"name": "quiet", "private": true}, nil)
	call("post_message", map[string]string{"text": "First line\nSecond line"}, nil)
	var listing struct{ Streams []string }
	call("list_streams", map[string]any{}, &listing)
	if len(listing.Streams) != 1 || listing.Streams[0] != DefaultStream {
		t.Fatal(listing)
	}
	var read struct{ Messages []*Message }
	call("read_messages", map[string]int{"limit": 1}, &read)
	if len(read.Messages) != 1 || read.Messages[0].Text != "First line\nSecond line" {
		t.Fatal(read)
	}
	call("post_message", map[string]string{"text": "Could you explain this? @malten"}, nil)
	select {
	case question := <-s.Commands:
		if question.Text != "Could you explain this?" || question.Stream != DefaultStream {
			t.Fatalf("wrong question: %+v", question)
		}
	default:
		t.Fatal("MCP mention did not reach the agent")
	}
}
