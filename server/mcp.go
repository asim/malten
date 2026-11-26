package server

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type readStreamArgs struct {
	Stream string `json:"stream,omitempty" jsonschema:"Stream name (defaults to the home stream)"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Number of recent messages (default 25)"`
}
type postMessageArgs struct {
	Parent string `json:"parent,omitempty" jsonschema:"ID of the reflection to quote in this stream"`
	Stream string `json:"stream,omitempty" jsonschema:"Stream name (defaults to the home stream)"`
	Text   string `json:"text" jsonschema:"Reflection to publish, up to 4096 bytes"`
}
type createStreamArgs struct {
	Name    string `json:"name,omitempty" jsonschema:"Optional stream name; generated if omitted"`
	Private bool   `json:"private,omitempty" jsonschema:"Hide this stream from the public listing"`
}

func MCPHandler(s *Server) http.Handler {
	api := mcp.NewServer(&mcp.Implementation{Name: "malten", Version: "1"}, nil)
	mcp.AddTool(api, &mcp.Tool{Name: "list_streams", Description: "List public stream names."}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, map[string][]string, error) {
		names := []string{}
		for name, stream := range s.List() {
			if !stream.Private {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		return nil, map[string][]string{"streams": names}, nil
	})
	mcp.AddTool(api, &mcp.Tool{Name: "read_messages", Description: "Read recent reflections in chronological order."}, func(ctx context.Context, req *mcp.CallToolRequest, args readStreamArgs) (*mcp.CallToolResult, any, error) {
		if args.Stream == "" {
			args.Stream = DefaultStream
		}
		if args.Limit == 0 {
			args.Limit = 25
		}
		if args.Limit < 1 || args.Limit > MaxMessages {
			return nil, nil, errors.New("limit must be between 1 and 1024")
		}
		messages := s.Retrieve("", args.Stream, 1, 0, int64(args.Limit))
		if messages == nil {
			messages = []*Message{}
		}
		return nil, map[string][]*Message{"messages": messages}, nil
	})
	mcp.AddTool(api, &mcp.Tool{Name: "post_message", Description: "Publish a reflection to a stream."}, func(ctx context.Context, req *mcp.CallToolRequest, args postMessageArgs) (*mcp.CallToolResult, map[string]string, error) {
		if strings.TrimSpace(args.Text) == "" || len(args.Text) > MaxMessageSize {
			return nil, nil, errors.New("text must be nonblank and at most 4096 bytes")
		}
		if args.Stream == "" {
			args.Stream = DefaultStream
		}
		message := NewMessage(args.Text, args.Stream)
		message.Parent = args.Parent
		if err := s.postMessage(message); err != nil {
			return nil, nil, err
		}
		return nil, map[string]string{"id": message.Id, "stream": message.Stream}, nil
	})
	mcp.AddTool(api, &mcp.Tool{Name: "create_stream", Description: "Create a stream with a rolling 24-hour TTL."}, func(ctx context.Context, req *mcp.CallToolRequest, args createStreamArgs) (*mcp.CallToolResult, map[string]string, error) {
		if args.Name == "" {
			args.Name = Random(8)
		}
		if err := s.New(args.Name, "", args.Private, int(StreamTTL/time.Second)); err != nil {
			return nil, nil, err
		}
		return nil, map[string]string{"stream": args.Name}, nil
	})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return api }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, MaxRequestBodyBytes: 1 << 20})
	return http.NewCrossOriginProtection().Handler(handler)
}
