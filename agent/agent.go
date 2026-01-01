package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/asim/malten/command"
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
	DefaultPrompt = `You are a helpful assistant with access to tools/services.

Available tools:
- price: Get cryptocurrency prices (e.g., btc, eth, sol)
- reminder: Get Islamic reminders (Quran, Hadith, Names of Allah) or search Islamic texts
- news: Get latest news headlines or search news
- video: Search for videos
- blog: Get latest blog posts
- chat: Ask questions with real-time news/video context

Use tools when appropriate. For general questions, answer directly.

Output rules:
- Be concise and direct
- Max 1024 chars

CRISIS EXCEPTION: If someone expresses self-harm, suicide, or severe distress, reply ONLY:
"samaritans.org - call 116 123 (UK) or find your local branch"`

	MaxTokens = 1024
)

// Tool definitions for the LLM
var tools = []openai.Tool{
	{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        "price",
			Description: "Get cryptocurrency price. Use for any question about crypto prices.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"coin": {
						"type": "string",
						"description": "Coin symbol or name (e.g., btc, eth, bitcoin, ethereum)"
					}
				},
				"required": ["coin"]
			}`),
		},
	},
	{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        "reminder",
			Description: "Get Islamic reminder (Quran verse, Hadith, Name of Allah) or search Islamic texts. Use for questions about Islam, Quran, Hadith, Allah.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {
						"type": "string",
						"description": "Optional search query. Leave empty for daily reminder."
					}
				}
			}`),
		},
	},
	{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        "news",
			Description: "Get latest news headlines or search news. Use for questions about current events, news, what's happening.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {
						"type": "string",
						"description": "Optional search query. Leave empty for latest headlines."
					}
				}
			}`),
		},
	},
	{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        "video",
			Description: "Search for videos. Use when user wants to watch or find videos.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {
						"type": "string",
						"description": "Video search query"
					}
				},
				"required": ["query"]
			}`),
		},
	},
	{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        "blog",
			Description: "Get latest blog posts.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {}
			}`),
		},
	},
	{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        "chat",
			Description: "Ask a question with real-time context from news and videos. Use for questions that need current/real-time information.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"question": {
						"type": "string",
						"description": "The question to ask"
					}
				},
				"required": ["question"]
			}`),
		},
	},
	{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        "nearby",
			Description: "Find nearby places like cafes, restaurants, pharmacies, etc. Requires user location to be enabled.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"type": {
						"type": "string",
						"description": "Type of place (cafe, restaurant, pharmacy, hospital, bank, atm, supermarket, shop, gas, parking, gym, mosque, church, hotel)"
					}
				},
				"required": ["type"]
			}`),
		},
	},
}

// Message represents a conversation message
type Message struct {
	Role    string
	Content string
}

// executeTool runs a tool and returns the result
func executeTool(name string, args map[string]interface{}) (string, error) {
	switch name {
	case "price":
		coin, _ := args["coin"].(string)
		if coin == "" {
			return "Please specify a coin", nil
		}
		return command.Execute("price", []string{coin})

	case "reminder":
		query, _ := args["query"].(string)
		if query == "" {
			return command.Execute("reminder", nil)
		}
		return command.Execute("reminder", []string{query})

	case "news":
		query, _ := args["query"].(string)
		if query == "" {
			return command.Execute("news", nil)
		}
		return command.Execute("news", []string{query})

	case "video":
		query, _ := args["query"].(string)
		if query == "" {
			return "Please specify a search query", nil
		}
		return command.Execute("video", []string{query})

	case "blog":
		return command.Execute("blog", nil)

	case "chat":
		question, _ := args["question"].(string)
		if question == "" {
			return "Please specify a question", nil
		}
		return command.Execute("chat", []string{question})

	case "nearby":
		placeType, _ := args["type"].(string)
		if placeType == "" {
			return "Please specify a place type (e.g., cafe, restaurant)", nil
		}
		// Check if we have location for the current stream
		loc := command.GetLocation(CurrentStream)
		if loc == nil {
			return "📍 Location not available. Enable location? Use /ping on", nil
		}
		return command.NearbyWithLocation(placeType, loc.Lat, loc.Lon)

	default:
		return "", errors.New("unknown tool: " + name)
	}
}

// ToolSelectionPrompt is used to ask the LLM which tool to use
var ToolSelectionPrompt = `You are a router that decides which tool to use.

Available tools:
- price: Get cryptocurrency prices. Use for: btc price, eth price, what's bitcoin worth, crypto prices
- reminder: Islamic content (Quran, Hadith, Names of Allah). Use for: Islamic questions, Quran verses, hadith, daily reminder
- news: News headlines and search. Use for: current events, what's happening, news about X
- video: Video search. Use for: find videos, watch tutorials, video about X
- blog: Blog posts. Use for: blog, posts, articles
- chat: Real-time AI with current context. Use for: questions needing current info, analysis with real-time data
- none: Direct answer without tools. Use for: general questions, math, definitions, coding help

Respond with ONLY a JSON object, nothing else:
{"tool": "toolname", "args": {"param": "value"}}

Examples:
- "what's the btc price" -> {"tool": "price", "args": {"coin": "btc"}}
- "show me golang videos" -> {"tool": "video", "args": {"query": "golang"}}
- "what is 2+2" -> {"tool": "none", "args": {}}
- "latest news" -> {"tool": "news", "args": {}}
- "news about gaza" -> {"tool": "news", "args": {"query": "gaza"}}
- "daily reminder" -> {"tool": "reminder", "args": {}}
- "what does quran say about patience" -> {"tool": "reminder", "args": {"query": "patience"}}`

// CurrentStream holds the stream context for the current request
var CurrentStream string

// Prompt sends a prompt to the AI with context and returns the response
func Prompt(systemPrompt string, messages []Message, userPrompt string) (string, error) {
	if Client == nil {
		return "", errors.New("AI client not initialized")
	}

	// Step 1: Ask LLM which tool to use
	decision, err := selectTool(userPrompt)
	if err == nil && decision != nil && decision.Tool != "none" && decision.Tool != "" {
		// Ensure args map exists
		if decision.Args == nil {
			decision.Args = make(map[string]interface{})
		}
		// For chat tool, ensure the question is passed through
		if decision.Tool == "chat" {
			if q, _ := decision.Args["question"].(string); q == "" {
				decision.Args["question"] = userPrompt
			}
		}
		// Execute the tool
		result, err := executeTool(decision.Tool, decision.Args)
		if err != nil {
			return "Error: " + err.Error(), nil
		}
		if result != "" {
			return result, nil
		}
	}

	// Step 2: No tool needed or tool failed, get direct response
	return directResponse(systemPrompt, messages, userPrompt)
}

type ToolDecision struct {
	Tool string                 `json:"tool"`
	Args map[string]interface{} `json:"args"`
}

func selectTool(userPrompt string) (*ToolDecision, error) {
	// Build tool selection prompt as user message (Fanar ignores system prompts for this)
	selectionPrompt := `Which tool should I use for this question: "` + userPrompt + `"

Available tools:
- price: cryptocurrency prices (btc, eth, etc)
- reminder: Islamic content (Quran, Hadith, daily reminder)
- news: news headlines or search
- video: search videos
- blog: blog posts
- nearby: find nearby places (cafes, restaurants, pharmacies, etc)
- chat: questions needing real-time current info
- none: general questions, math, coding, definitions

Respond ONLY with JSON: {"tool": "name", "args": {"key": "value"}}
Examples:
- btc price -> {"tool": "price", "args": {"coin": "btc"}}
- news about AI -> {"tool": "news", "args": {"query": "AI"}}
- cafes nearby -> {"tool": "nearby", "args": {"type": "cafe"}}
- restaurants near me -> {"tool": "nearby", "args": {"type": "restaurant"}}
- what is 2+2 -> {"tool": "none", "args": {}}`

	resp, err := Client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: ModelName,
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleUser, Content: selectionPrompt},
			},
			MaxTokens: 100,
		},
	)
	if err != nil {
		return nil, err
	}

	if len(resp.Choices) == 0 {
		return nil, errors.New("no response")
	}

	content := resp.Choices[0].Message.Content
	
	// Parse JSON from response
	var decision ToolDecision
	if err := json.Unmarshal([]byte(content), &decision); err != nil {
		// Try to extract JSON from response
		start := strings.Index(content, "{")
		end := strings.LastIndex(content, "}")
		if start >= 0 && end > start {
			json.Unmarshal([]byte(content[start:end+1]), &decision)
		}
	}

	return &decision, nil
}

func directResponse(systemPrompt string, messages []Message, userPrompt string) (string, error) {
	chatMessages := []openai.ChatCompletionMessage{{
		Role:    openai.ChatMessageRoleSystem,
		Content: systemPrompt,
	}}

	for _, m := range messages {
		chatMessages = append(chatMessages, openai.ChatCompletionMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

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
