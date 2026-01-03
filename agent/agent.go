package agent

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"strings"

	"malten.ai/command"
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
	DefaultPrompt = `You are a spatial assistant. Be extremely concise.

The user's LIVE LOCATION CONTEXT is below. Use it to answer.

Response format:
- "Where am I" → Just the address, nothing else
- "Weather" → Just temp and conditions
- "Next bus" → Just route and time
- "Cafes" → List 2-3 names only
- General "what's around" → Pick ONE interesting thing to mention

Rules:
- MAX 1-2 sentences
- NO prose, NO filler words
- NO "You are at..." or "The weather is..."
- Just facts: "Milton Road, TW12. 1°C. 281 to Kingston in 7m."
- NEVER repeat the entire context back
- NEVER list every single place

Tools (use ONLY when context doesn't have the answer):
- price: Crypto prices
- reminder: Islamic reminders
- news: News search
- video: Video search

CRISIS: Self-harm/suicide → reply ONLY: "samaritans.org - 116 123 (UK)"`

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
			Description: "Find nearby places. Use for any 'X near me' or 'find X' query where X is a place type.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"type": {
						"type": "string",
						"description": "What to search for (bowling, cinema, gym, hotel, arcade, spa, or any place type)"
					}
				},
				"required": ["type"]
			}`),
		},
	},
	{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        "directions",
			Description: "Get walking directions to a destination. Use for 'how do I get to X', 'directions to X', 'walk to X' type questions.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"destination": {
						"type": "string",
						"description": "Where to go (e.g., 'the station', 'Tesco', 'Whitton Station')"
					}
				},
				"required": ["destination"]
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

	case "directions":
		dest, _ := args["destination"].(string)
		log.Printf("[directions] destination=%q CurrentStream=%q", dest, CurrentStream)
		if dest == "" {
			return "Where do you want to go?", nil
		}
		loc := command.GetLocation(CurrentStream)
		log.Printf("[directions] loc=%v", loc)
		if loc == nil {
			return "📍 Need your location for directions. Enable location?", nil
		}
		result, err := command.Directions(dest, loc.Lat, loc.Lon)
		log.Printf("[directions] result=%d chars, err=%v", len(result), err)
		return result, err

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
- nearby: Find nearby places. Use for: X near me, find X, where's the nearest X
- directions: Walking directions. Use for: how do I get to X, directions to X, walk to X, way to X
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
- "what does quran say about patience" -> {"tool": "reminder", "args": {"query": "patience"}}
- "how do I get to the station" -> {"tool": "directions", "args": {"destination": "station"}}
- "directions to tesco" -> {"tool": "directions", "args": {"destination": "tesco"}}
- "cafes near me" -> {"tool": "nearby", "args": {"type": "cafe"}}`

// CurrentStream holds the stream context for the current request
var CurrentStream string

// Prompt sends a prompt to the AI with context and returns the response
func Prompt(systemPrompt string, messages []Message, userPrompt string) (string, error) {
	if Client == nil {
		return "", errors.New("AI client not initialized")
	}

	hasContext := strings.Contains(systemPrompt, "📍")
	
	// Check for directions - still need tool for routing even with context
	lower := strings.ToLower(userPrompt)
	needsDirections := strings.Contains(lower, "how do i get") || strings.Contains(lower, "directions to") ||
		strings.Contains(lower, "walk to") || strings.Contains(lower, "route to")
	
	if needsDirections {
		log.Printf("[AI] Directions query, extracting destination")
		// Extract destination directly without going through selectTool
		dest := userPrompt
		for _, prefix := range []string{"how do i get to ", "how do I get to ", "directions to ", "walk to ", "way to ", "route to "} {
			if idx := strings.Index(lower, prefix); idx >= 0 {
				dest = userPrompt[idx+len(prefix):]
				break
			}
		}
		dest = strings.TrimSuffix(dest, "?")
		dest = strings.TrimSpace(dest)
		if dest != "" {
			log.Printf("[AI] Directions to: %q", dest)
			result, err := executeTool("directions", map[string]interface{}{"destination": dest})
			if err == nil && result != "" {
				return result, nil
			}
		}
	}

	// If we have location context, let LLM answer directly
	if hasContext {
		log.Printf("[AI] Has context, using direct response")
		return directResponse(systemPrompt, messages, userPrompt)
	}

	// No context - use tool selection for things like price, news, etc
	decision, err := selectTool(userPrompt)
	if err == nil && decision != nil && decision.Tool != "none" && decision.Tool != "" {
		if decision.Args == nil {
			decision.Args = make(map[string]interface{})
		}
		if decision.Tool == "chat" {
			if q, _ := decision.Args["question"].(string); q == "" {
				decision.Args["question"] = userPrompt
			}
		}
		result, err := executeTool(decision.Tool, decision.Args)
		if err != nil {
			return "Error: " + err.Error(), nil
		}
		if result != "" {
			return result, nil
		}
	}

	return directResponse(systemPrompt, messages, userPrompt)
}

type ToolDecision struct {
	Tool string                 `json:"tool"`
	Args map[string]interface{} `json:"args"`
}

// Types we show in context (and can answer from context)
var contextPlaceTypes = map[string]bool{
	"cafe": true, "cafes": true, "coffee": true,
	"restaurant": true, "restaurants": true,
	"pharmacy": true, "pharmacies": true,
	"supermarket": true, "supermarkets": true, "grocery": true,
	"shop": true, "shops": true,
}

// isContextQuestion returns true if the question should be answered from location context
func isContextQuestion(prompt string) bool {
	lower := strings.ToLower(prompt)
	
	// Always-in-context: location, weather, buses, prayer
	alwaysContext := []string{
		"where am i", "my location", "what is this", "what's this",
		"next bus", "bus time", "when is the bus", "train time",
		"weather", "temperature", "cold", "hot", "rain",
		"prayer", "fajr", "dhuhr", "asr", "maghrib", "isha",
		"what's around", "what is around", "what's happening",
	}
	for _, kw := range alwaysContext {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	
	// For "near me"/"nearby" queries, only match if it's a type we have in context
	// "cafes near me" → context, "bowling near me" → tool
	if strings.Contains(lower, "near me") || strings.Contains(lower, "nearby") || 
	   strings.Contains(lower, "around me") || strings.Contains(lower, "close by") {
		words := strings.Fields(lower)
		for _, w := range words {
			if contextPlaceTypes[w] {
				return true
			}
		}
		// Has "near me" but no recognized context type → needs tool
		return false
	}
	
	// Direct place type mention without "near me" (e.g., just "cafes?")
	words := strings.Fields(lower)
	for _, w := range words {
		w = strings.Trim(w, "?!.,")
		if contextPlaceTypes[w] {
			return true
		}
	}
	
	return false
}

func selectTool(userPrompt string) (*ToolDecision, error) {
	// If it's a location/context question, use direct response (none tool)
	if isContextQuestion(userPrompt) {
		log.Printf("[tool] isContextQuestion=true for %q", userPrompt)
		return &ToolDecision{Tool: "none", Args: map[string]interface{}{}}, nil
	}
	log.Printf("[tool] isContextQuestion=false for %q - will ask LLM", userPrompt)

	// Check for directions keywords first - LLM often gets this wrong
	lower := strings.ToLower(userPrompt)
	if strings.Contains(lower, "how do i get to") || strings.Contains(lower, "directions to") ||
		strings.Contains(lower, "walk to") || strings.Contains(lower, "way to the") ||
		strings.Contains(lower, "route to") || (strings.Contains(lower, "get to") && strings.Contains(lower, "how")) {
		// Extract destination
		dest := userPrompt
		for _, prefix := range []string{"how do i get to ", "how do I get to ", "directions to ", "walk to ", "way to ", "route to ", "get to "} {
			if idx := strings.Index(lower, prefix); idx >= 0 {
				dest = userPrompt[idx+len(prefix):]
				break
			}
		}
		dest = strings.TrimSuffix(dest, "?")
		dest = strings.TrimSpace(dest)
		log.Printf("[tool] Detected directions question, dest=%q", dest)
		return &ToolDecision{Tool: "directions", Args: map[string]interface{}{"destination": dest}}, nil
	}

	// Build tool selection prompt as user message (Fanar ignores system prompts for this)
	selectionPrompt := `Which tool should I use for this question: "` + userPrompt + `"

Available tools:
- price: cryptocurrency prices (btc, eth, etc)
- reminder: Islamic content (Quran, Hadith, daily reminder)
- news: news headlines or search news
- video: search videos
- nearby: find places near user (bowling, cinema, gym, hotel, any place type)
- directions: walking directions to a place (how do I get to X, directions to X)
- none: general questions, math, coding, conversation

Respond ONLY with JSON: {"tool": "name", "args": {"key": "value"}}
Examples:
- btc price -> {"tool": "price", "args": {"coin": "btc"}}
- news about AI -> {"tool": "news", "args": {"query": "AI"}}
- search hadith about patience -> {"tool": "reminder", "args": {"query": "patience"}}
- bowling near me -> {"tool": "nearby", "args": {"type": "bowling"}}
- find a cinema -> {"tool": "nearby", "args": {"type": "cinema"}}
- gyms nearby -> {"tool": "nearby", "args": {"type": "gym"}}
- how do I get to the station -> {"tool": "directions", "args": {"destination": "station"}}
- hello -> {"tool": "none", "args": {}}
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
	log.Printf("[tool] LLM response: %s", content)
	
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
	log.Printf("[tool] Parsed decision: tool=%s args=%v", decision.Tool, decision.Args)

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

	// For Fanar: include context reminder since it may ignore system prompt
	userMsg := userPrompt
	if strings.Contains(systemPrompt, "📍") {
		// Has location context - remind LLM to use it
		userMsg = userPrompt + "\n\n(Answer from the location context above. Don't search the web. Be concise.)"
	}

	chatMessages = append(chatMessages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: userMsg,
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
