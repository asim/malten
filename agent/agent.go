package agent

import (
	"context"
	"errors"
	"os"

	"github.com/sashabaranov/go-openai"
)

var (
	Key       = os.Getenv("OPENAI_API_KEY")
	FanarKey  = os.Getenv("FANAR_API_KEY")
	FanarURL  = os.Getenv("FANAR_API_URL")
	ModelName = openai.GPT3Dot5Turbo

	Client *openai.Client
)

var (
	DefaultPrompt = `Utility assistant only. Strict rules:

DO: summarize, translate, explain facts, calculate, help with code, format text.

DO NOT: give advice, opinions, life guidance, therapy, personal counsel, motivational talk, or roleplay. No exceptions.

For any personal question, advice request, or "what should I" question, reply ONLY: "I help with factual questions and practical tasks only."

Be brief. Max 300 characters when possible.`

	MaxTokens = 1024
)

// Message represents a conversation message
type Message struct {
	Role    string
	Content string
}

// Prompt sends a prompt to the AI with context and returns the response
func Prompt(systemPrompt string, messages []Message, userPrompt string) (string, error) {
	if Client == nil {
		return "", errors.New("AI client not initialized")
	}

	// Build chat messages
	chatMessages := []openai.ChatCompletionMessage{{
		Role:    openai.ChatMessageRoleSystem,
		Content: systemPrompt,
	}}

	// Add context messages
	for _, m := range messages {
		chatMessages = append(chatMessages, openai.ChatCompletionMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	// Add the user prompt
	chatMessages = append(chatMessages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: userPrompt,
	})

	resp, err := Client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model:     ModelName,
			Messages:  chatMessages,
			MaxTokens: MaxTokens,
		},
	)
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", errors.New("no response from AI")
	}

	return resp.Choices[0].Message.Content, nil
}

// Init initializes the AI client
func Init() error {
	// Prefer Fanar if configured
	if len(FanarKey) > 0 && len(FanarURL) > 0 {
		config := openai.DefaultConfig(FanarKey)
		config.BaseURL = FanarURL
		Client = openai.NewClientWithConfig(config)
		ModelName = "Fanar"
		return nil
	}

	if len(Key) > 0 {
		Client = openai.NewClient(Key)
		ModelName = openai.GPT3Dot5Turbo
		return nil
	}

	return errors.New("missing OPENAI_API_KEY or FANAR_API_KEY")
}
