// Package reminder shares reflections from reminder.dev.
package reminder

import (
	"context"
	"encoding/json"
	"github.com/asim/malten/agent"
	"io"
	"net/http"
	"strings"
	"time"
)

var Streams = []agent.Stream{{Tag: "reminder"}}

func Run(ctx context.Context, publish agent.PublishPhoto) {
	agent.RunSource(ctx, "reminder", Fetch, publish)
}

func latest(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", "https://reminder.dev/api/latest", nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", nil
	}
	var reminder struct {
		Message string `json:"message"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 128*1024)).Decode(&reminder); err != nil {
		return "", err
	}
	message := strings.TrimSpace(reminder.Message)
	if message == "" || len([]rune(message)) > 1100 {
		return "", nil
	}
	// Generated prose is explicitly attributed; scripture is not rewritten.
	return message + "\n\nhttps://reminder.dev", nil
}

func Fetch(ctx context.Context, _ time.Time) (agent.Post, error) {
	text, err := latest(ctx)
	return agent.Post{Text: text, Name: "Reminder · AI reflection"}, err
}
