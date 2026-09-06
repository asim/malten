package aslam

import (
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/asim/malten/agent"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

//go:embed photos/*.jpg
var photos embed.FS

var Streams = []agent.Stream{
	{Tag: "sunrise"},
	{Tag: "morning"},
	{Tag: "afternoon"},
	{Tag: "sunset"},
	{Tag: "evening"},
}

// Run keeps a small set of sourced reminders available across time zones.
// Browsers select a theme using local time; these are not astronomical times.
func Run(ctx context.Context, publish agent.PublishPhoto) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			for i, stream := range Streams {
				if ctx.Err() != nil {
					return
				}
				query, role := "morning", "daily dua"
				if i < 2 {
					query, role = "blessings", "morning dhikr"
				}
				if stream.Tag == "sunset" || stream.Tag == "evening" {
					query, role = "evening", "evening dhikr"
				}
				text, err := aslamReminder(ctx, query, role, time.Now().UTC().YearDay()+i)
				if err == nil {
					file := "photos/trees.jpg"
					if i < 2 {
						file = "photos/sunrise.jpg"
					}
					raw, _ := photos.ReadFile(file)
					err = publish(ctx, stream.Tag, text, "Aslam · adhkar", "data:image/jpeg;base64,"+base64.StdEncoding.EncodeToString(raw), time.Now().UTC().Format("2006-01-02"))
				}
				if err != nil && ctx.Err() == nil {
					log.Printf("aslam: %s reminder unavailable", stream.Tag)
				}
			}
			timer.Reset(6 * time.Hour)
		}
	}
}

func aslamReminder(ctx context.Context, query, role string, pick int) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", "https://aslam.org/api/search?q="+url.QueryEscape(query), nil)
	if err != nil {
		return "", err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", fmt.Errorf("search status %d", res.StatusCode)
	}
	return readReminder(io.LimitReader(res.Body, 512*1024), role, pick)
}

func readReminder(r io.Reader, role string, pick int) (string, error) {
	var response struct {
		Results []struct{ Content, Kind, Role, Title, URL string }
	}
	if err := json.NewDecoder(r).Decode(&response); err != nil {
		return "", err
	}
	var choices []string
	for _, item := range response.Results {
		content := strings.TrimSpace(item.Content)
		// Search excerpts may be truncated. Never reconstruct or rewrite scripture.
		if item.Kind != "adhkar" || item.Role != role || content == "" || strings.HasSuffix(content, "...") || strings.HasSuffix(content, "…") {
			continue
		}
		u, err := url.Parse(item.URL)
		if err != nil || u.IsAbs() || u.Host != "" || !strings.HasPrefix(u.Path, "/adhkar/") {
			continue
		}
		text := content + "\n\n" + item.Title + "\nhttps://aslam.org" + u.String()
		if len([]rune(text)) <= 1200 {
			choices = append(choices, text)
		}
	}
	if len(choices) == 0 {
		return "", fmt.Errorf("no complete adhkar found")
	}
	return choices[pick%len(choices)], nil
}

// Fetch selects a sourced reminder for the viewer's local part of the day.
func Fetch(ctx context.Context, local time.Time) (agent.Post, error) {
	query, role := "morning", "daily dua"
	if local.Hour() >= 5 && local.Hour() < 12 {
		query, role = "blessings", "morning dhikr"
	}
	if local.Hour() >= 18 || local.Hour() < 5 {
		query, role = "evening", "evening dhikr"
	}
	text, err := aslamReminder(ctx, query, role, local.YearDay()+local.Hour())
	if err != nil {
		return agent.Post{}, err
	}
	file := "photos/trees.jpg"
	if local.Hour() >= 5 && local.Hour() < 12 {
		file = "photos/sunrise.jpg"
	}
	raw, err := photos.ReadFile(file)
	return agent.Post{Text: text, Name: "Aslam · adhkar", Photo: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(raw)}, err
}
