package news

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/asim/malten/agent"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Fetch supplies fresh topic-based headlines during waking hours.
func Fetch(ctx context.Context, local time.Time) (agent.Post, error) {
	if local.Hour() < 7 || local.Hour() >= 21 {
		return agent.Post{}, nil
	}
	text, err := newsHeadlines(ctx)
	return agent.Post{Text: text, Name: "News · Micro"}, err
}

func newsHeadlines(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", "https://micro.mu/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"news_headlines","arguments":{}}}`))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", errors.New("headlines request failed")
	}
	return readHeadlines(io.LimitReader(res.Body, 256*1024))
}

func readHeadlines(reader io.Reader) (string, error) {
	var response struct {
		Error  json.RawMessage
		Result struct {
			IsError bool
			Content []struct{ Type, Text string }
		}
	}
	if err := json.NewDecoder(reader).Decode(&response); err != nil {
		return "", err
	}
	if (len(response.Error) > 0 && string(response.Error) != "null") || response.Result.IsError {
		return "", errors.New("headlines tool failed")
	}
	text := "Headlines"
	seen := map[string]bool{}
	count := 0
	for _, content := range response.Result.Content {
		if content.Type != "text" {
			continue
		}
		var headlines struct {
			Items []struct{ Category, Title, URL string }
		}
		if err := json.Unmarshal([]byte(content.Text), &headlines); err != nil {
			return "", err
		}
		for _, item := range headlines.Items {
			category, title := strings.TrimSpace(item.Category), strings.TrimSpace(item.Title)
			link, err := url.Parse(item.URL)
			if err != nil || link.Host == "" || (link.Scheme != "https" && link.Scheme != "http") || link.User != nil {
				continue
			}
			if category == "" || title == "" || seen[strings.ToLower(category)] {
				continue
			}
			entry := "\n\n" + category + " · " + title + "\n" + link.String()
			if len([]rune(text+entry)) > 1200 {
				continue
			}
			text += entry
			seen[strings.ToLower(category)] = true
			count++
			if count == 3 {
				return text, nil
			}
		}
	}
	if count == 0 {
		return "", errors.New("no usable headlines")
	}
	return text, nil
}
