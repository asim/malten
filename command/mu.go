package command

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

const muBaseURL = "https://mu.xyz"

var muToken string

func init() {
	muToken = os.Getenv("MU_API_TOKEN")

	Register(&Command{
		Name:        "chat",
		Description: "Ask AI with real-time news/video context",
		Usage:       "chat <question>",
		Handler:     chatHandler,
	})

	Register(&Command{
		Name:        "news",
		Description: "Get latest news from Mu",
		Usage:       "news [query]",
		Handler:     newsHandler,
	})

	Register(&Command{
		Name:        "video",
		Description: "Search videos on Mu",
		Usage:       "video <query>",
		Handler:     videoHandler,
	})

	Register(&Command{
		Name:        "blog",
		Description: "Get latest blog posts from Mu",
		Usage:       "blog",
		Handler:     blogHandler,
	})
}

type MuChatResponse struct {
	Answer string `json:"answer"`
	Prompt string `json:"prompt"`
	Topic  string `json:"topic"`
}

func chatHandler(ctx *Context, args []string) (string, error) {
	if muToken == "" {
		return "Mu API token not configured", nil
	}

	if len(args) == 0 {
		return "Usage: chat <question>", nil
	}

	prompt := strings.Join(args, " ")

	// Use longer timeout for AI chat
	resp, err := muRequestWithTimeout("POST", "/chat", map[string]string{"prompt": prompt}, 60*time.Second)
	if err != nil {
		return "Error contacting Mu", err
	}
	defer resp.Body.Close()

	var data MuChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "Error parsing response", err
	}

	if data.Answer == "" {
		return "No response from Mu", nil
	}

	// Strip HTML tags from answer
	answer := stripHTML(data.Answer)

	return answer, nil
}

type NewsItem struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Content     string `json:"content"`
	URL         string `json:"url"`
}

type NewsResponse struct {
	Feed    []NewsItem `json:"feed"`
	Results []NewsItem `json:"results"`
	Count   int        `json:"count"`
}

type VideoResult struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Channel   string `json:"channel"`
	Type      string `json:"type"`
	Thumbnail string `json:"thumbnail"`
}

type VideoResponse struct {
	Results []VideoResult `json:"results"`
}

type BlogPost struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Author    string `json:"author"`
	CreatedAt string `json:"created_at"`
}

func muRequest(method, endpoint string, body interface{}) (*http.Response, error) {
	return muRequestWithTimeout(method, endpoint, body, 15*time.Second)
}

func muRequestWithTimeout(method, endpoint string, body interface{}, timeout time.Duration) (*http.Response, error) {
	var req *http.Request
	var err error

	if body != nil {
		jsonBody, _ := json.Marshal(body)
		req, err = http.NewRequest(method, muBaseURL+endpoint, bytes.NewBuffer(jsonBody))
	} else {
		req, err = http.NewRequest(method, muBaseURL+endpoint, nil)
	}
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if muToken != "" {
		req.Header.Set("Authorization", "Bearer "+muToken)
	}

	client := &http.Client{Timeout: timeout}
	return client.Do(req)
}

func newsHandler(ctx *Context, args []string) (string, error) {
	if muToken == "" {
		return "Mu API token not configured", nil
	}

	var resp *http.Response
	var err error

	if len(args) > 0 {
		// Search news
		query := strings.Join(args, " ")
		resp, err = muRequest("POST", "/news", map[string]string{"query": query})
	} else {
		// Get latest news
		resp, err = muRequest("GET", "/news", nil)
	}
	if err != nil {
		return "Error fetching news", err
	}
	defer resp.Body.Close()

	var data NewsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "Error parsing response", err
	}

	items := data.Feed
	if len(data.Results) > 0 {
		items = data.Results
	}

	if len(items) == 0 {
		return "No news found", nil
	}

	// Get one item from each category for diversity
	var result strings.Builder
	result.WriteString("📰 NEWS\n\n")

	seen := make(map[string]bool)
	count := 0
	for _, item := range items {
		if seen[item.Category] {
			continue
		}
		seen[item.Category] = true
		count++

		result.WriteString(fmt.Sprintf("[%s] %s\n", item.Category, item.Title))
		if item.Description != "" {
			result.WriteString(item.Description + "\n")
		}
		result.WriteString(fmt.Sprintf("https://mu.xyz/news?id=%s\n\n", item.ID))

		if count >= 8 {
			break
		}
	}

	return strings.TrimSpace(result.String()), nil
}

func videoHandler(ctx *Context, args []string) (string, error) {
	if muToken == "" {
		return "Mu API token not configured", nil
	}

	if len(args) == 0 {
		return "Usage: video <query>", nil
	}

	query := strings.Join(args, " ")
	resp, err := muRequest("POST", "/video", map[string]string{"query": query})
	if err != nil {
		return "Error searching videos", err
	}
	defer resp.Body.Close()

	var data VideoResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "Error parsing response", err
	}

	if len(data.Results) == 0 {
		return "No videos found", nil
	}

	// Show top 5 videos
	var result strings.Builder
	result.WriteString("🎬 VIDEOS\n\n")

	max := 5
	if len(data.Results) < max {
		max = len(data.Results)
	}

	for i := 0; i < max; i++ {
		v := data.Results[i]
		if v.Type == "channel" {
			continue
		}
		result.WriteString(fmt.Sprintf("%s\n", v.Title))
		result.WriteString(fmt.Sprintf("by %s\n", v.Channel))
		result.WriteString(fmt.Sprintf("https://mu.xyz/video?id=%s\n\n", v.ID))
	}

	return strings.TrimSpace(result.String()), nil
}

func blogHandler(ctx *Context, args []string) (string, error) {
	if muToken == "" {
		return "Mu API token not configured", nil
	}

	resp, err := muRequest("GET", "/blog", nil)
	if err != nil {
		return "Error fetching blog", err
	}
	defer resp.Body.Close()

	var posts []BlogPost
	if err := json.NewDecoder(resp.Body).Decode(&posts); err != nil {
		return "Error parsing response", err
	}

	if len(posts) == 0 {
		return "No blog posts found", nil
	}

	// Show top 5 posts
	var result strings.Builder
	result.WriteString("📝 BLOG\n\n")

	max := 5
	if len(posts) < max {
		max = len(posts)
	}

	for i := 0; i < max; i++ {
		p := posts[i]
		result.WriteString(fmt.Sprintf("%s\n", p.Title))
		result.WriteString(fmt.Sprintf("by %s\n", p.Author))
		// Truncate content
		content := p.Content
		if len(content) > 100 {
			content = content[:100] + "..."
		}
		result.WriteString(content + "\n")
		result.WriteString(fmt.Sprintf("https://mu.xyz/post?id=%s\n\n", p.ID))
	}

	return strings.TrimSpace(result.String()), nil
}
