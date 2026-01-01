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

	// News is useful for spatial context
	Register(&Command{
		Name:        "news",
		Description: "Get latest news",
		Usage:       "news [query]",
		Handler:     newsHandler,
	})
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
		return "News not available", nil
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
