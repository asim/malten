package news

import (
	"encoding/json"
	"net/url"
	"time"

	"github.com/asim/malten/agent"
)

// Context keeps each retained headline attached to its original article link.
func Context(record agent.Record) []agent.Source {
	var wrapper struct {
		Data struct {
			Result struct{ Content []struct{ Type, Text string } }
		}
	}
	if json.Unmarshal(record.Data, &wrapper) != nil {
		return nil
	}
	var out []agent.Source
	for _, c := range wrapper.Data.Result.Content {
		if c.Type != "text" {
			continue
		}
		var data struct {
			Items []struct{ Title, URL, Category string }
		}
		if json.Unmarshal([]byte(c.Text), &data) != nil {
			continue
		}
		for _, item := range data.Items {
			u, err := url.Parse(item.URL)
			if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || item.Title == "" || len(item.Title) > 1000 {
				continue
			}
			out = append(out, agent.NewSource(item.Title, u.String(), "Headline only: "+item.Title+". Retrieved "+record.At.UTC().Format(time.RFC3339), true))
			if len(out) == 8 {
				return out
			}
		}
	}
	return out
}
