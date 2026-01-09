package command

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func init() {
	Register(&Command{
		Name:        "video",
		Description: "Search for videos",
		Usage:       "/video <query>",
		Emoji:       "🎬",
		LoadingText: "Searching videos...",
		Handler:     handleVideo,
	})
}

type videoResult struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Channel   string    `json:"channel"`
	Published time.Time `json:"published"`
	URL       string    `json:"url"`
	Thumbnail string    `json:"thumbnail"`
}

type videoResponse struct {
	Results []videoResult `json:"results"`
}

func handleVideo(ctx *Context, args []string) (string, error) {
	if len(args) == 0 {
		return "Usage: /video <search query>", nil
	}

	query := strings.Join(args, " ")

	// Call Mu's video API
	muURL := fmt.Sprintf("https://mu.xyz/video?query=%s", url.QueryEscape(query))

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("POST", "https://mu.xyz/video", strings.NewReader(fmt.Sprintf(`{"query":"%s"}`, query)))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to search videos: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		// Mu requires auth for search - fall back to showing link
		return fmt.Sprintf("🎬 Search videos: [%s](%s)", query, muURL), nil
	}

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("video search failed: %s", string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var vr videoResponse
	if err := json.Unmarshal(body, &vr); err != nil {
		// Mu requires auth - show link instead
		return fmt.Sprintf("🎬 Search videos: [%s](%s)", query, muURL), nil
	}

	if len(vr.Results) == 0 {
		return fmt.Sprintf("No videos found for '%s'", query), nil
	}

	// Format results
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🎬 **Videos: %s**\n\n", query))

	for i, v := range vr.Results {
		if i >= 5 {
			break
		}
		// Use YouTube URL directly
		ytURL := fmt.Sprintf("https://youtube.com/watch?v=%s", v.ID)
		sb.WriteString(fmt.Sprintf("**[%s](%s)**\n", v.Title, ytURL))
		if v.Channel != "" {
			sb.WriteString(fmt.Sprintf("  %s\n", v.Channel))
		}
		sb.WriteString("\n")
	}

	return sb.String(), nil
}
