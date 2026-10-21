package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/asim/malten/server"
	"github.com/sashabaranov/go-openai"
)

const DefaultModel = "gpt-6-astra"
const DefaultPersona = "Listen. Be patient. Respond kindly. Treat the messages as a continuing conversation. Keep replies concise."

var slots = make(chan struct{}, 4)

func requestFor(messages []*server.Message) openai.ChatCompletionRequest {
	parts := []openai.ChatCompletionMessage{{Role: openai.ChatMessageRoleSystem, Content: DefaultPersona}}
	// A rolling window, rebuilt from the stream instead of retaining a second history.
	start := len(messages) - 40
	if start < 0 {
		start = 0
	}
	used, photos := 0, 0
	recent := []openai.ChatCompletionMessage{}
	for i := len(messages) - 1; i >= start; i-- {
		m := messages[i]
		if used+len(m.Text) > 32000 {
			break
		}
		used += len(m.Text)
		role := openai.ChatMessageRoleUser
		if m.Role == "assistant" {
			role = openai.ChatMessageRoleAssistant
		}
		text := strings.TrimPrefix(m.Text, "/malten ")
		part := openai.ChatCompletionMessage{Role: role, Content: text}
		if m.Image != "" && photos < 2 && role == openai.ChatMessageRoleUser {
			photos++
			part.Content = ""
			part.MultiContent = []openai.ChatMessagePart{{Type: openai.ChatMessagePartTypeText, Text: text}, {Type: openai.ChatMessagePartTypeImageURL, ImageURL: &openai.ChatMessageImageURL{URL: m.Image}}}
		}
		recent = append(recent, part)
	}
	for i := len(recent) - 1; i >= 0; i-- {
		parts = append(parts, recent[i])
	}
	return openai.ChatCompletionRequest{Model: DefaultModel, Messages: parts, MaxCompletionTokens: 4096, ReasoningEffort: "low"}
}

// Reply is stateless. No stream name, account identity, or history is retained here.
func Reply(ctx context.Context, messages []*server.Message) (string, error) {
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		return "", errors.New("AI is not configured on this server.")
	}
	select {
	case slots <- struct{}{}:
		defer func() { <-slots }()
	default:
		return "", errors.New("AI is busy. Try again shortly.")
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	response, err := openai.NewClient(key).CreateChatCompletion(ctx, requestFor(messages))
	if err != nil {
		return "", errors.New("AI could not reply. Try again shortly.")
	}
	if len(response.Choices) == 0 || strings.TrimSpace(response.Choices[0].Message.Content) == "" {
		return "", errors.New("AI returned no reply. Try again.")
	}
	return response.Choices[0].Message.Content, nil
}

// PrivateHandler only relays the supplied context; it never touches server streams.
func PrivateHandler(w http.ResponseWriter, r *http.Request) {
	privateHandler(w, r, Reply)
}

func privateHandler(w http.ResponseWriter, r *http.Request, answer func(context.Context, []*server.Message) (string, error)) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != "POST" {
		http.Error(w, "Use POST", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	var input struct {
		Messages []*server.Message `json:"messages"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || len(input.Messages) == 0 || len(input.Messages) > 40 {
		http.Error(w, "Invalid context", 400)
		return
	}
	bytes := 0
	for _, m := range input.Messages {
		if m == nil {
			http.Error(w, "Invalid message", 400)
			return
		}
		bytes += len(m.Text)
		if bytes > 32000 || (m.Role != "user" && m.Role != "assistant") || server.ValidatePhoto(m.Image) != nil {
			http.Error(w, "Invalid context", 400)
			return
		}
	}
	text, err := answer(r.Context(), input.Messages)
	if err != nil {
		http.Error(w, err.Error(), 503)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"text": text})
}
