package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func MuAvailable() bool { return strings.TrimSpace(os.Getenv("MU_API_KEY")) != "" }

// MuCall only exposes public, read-only context. It never discovers or grants
// the model access to the account's mail, files, tasks or other private tools.
func MuCall(ctx context.Context, name string, args any) (string, error) {
	switch name {
	case "news_list", "news_search", "prayer_search", "places_geocode", "weather_forecast":
	default:
		return "", errors.New("Mu tool not allowed")
	}
	key := strings.TrimSpace(os.Getenv("MU_API_KEY"))
	if key == "" {
		return "", errors.New("Mu key unavailable")
	}
	raw, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", "https://micro.mu/mcp", strings.NewReader(string(raw)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	client := *http.DefaultClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := client.Do(req)
	if err != nil {
		return "", errors.New("Mu unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", errors.New("Mu request failed")
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, (128<<10)+1))
	if err != nil || len(data) > 128<<10 {
		return "", errors.New("invalid Mu response")
	}
	var envelope struct {
		Error  json.RawMessage
		Result struct {
			IsError bool
			Content []struct{ Type, Text string }
		}
	}
	if json.Unmarshal(data, &envelope) != nil || len(envelope.Error) > 0 && string(envelope.Error) != "null" || envelope.Result.IsError {
		return "", errors.New("Mu tool failed")
	}
	var parts []string
	for _, c := range envelope.Result.Content {
		if c.Type == "text" {
			parts = append(parts, c.Text)
		}
	}
	text := strings.TrimSpace(strings.Join(parts, "\n"))
	if text == "" {
		return "", errors.New("empty Mu response")
	}
	return text, nil
}
