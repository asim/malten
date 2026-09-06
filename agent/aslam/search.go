package aslam

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/asim/malten/agent"
)

// Search retains attributed excerpts. They are not complete quotations.
func Search(ctx context.Context, query string) ([]agent.Source, error) {
	raw, err := agent.ReadJSON(ctx, "GET", "https://aslam.org/api/search?q="+url.QueryEscape(query), "")
	if err != nil {
		return nil, err
	}
	var data struct {
		Results []struct{ Title, URL, Content, Kind string }
	}
	if json.Unmarshal(raw, &data) != nil || data.Results == nil {
		return nil, errors.New("invalid Aslam search")
	}
	base, _ := url.Parse("https://aslam.org")
	out := []agent.Source{}
	size := 0
	counts := map[string]int{}
	for _, r := range data.Results {
		if r.URL == "" || r.URL == "#" || strings.TrimSpace(r.Content) == "" || counts[r.Kind] >= 2 {
			continue
		}
		u, err := url.Parse(r.URL)
		if err != nil {
			continue
		}
		u = base.ResolveReference(u)
		if u.Scheme != "https" || u.Host != "aslam.org" || u.User != nil {
			continue
		}
		text := []rune(r.Content)
		if len(text) > 1500 {
			text = text[:1500]
		}
		if size+len(string(text)) > 12000 {
			continue
		}
		size += len(string(text))
		out = append(out, agent.NewSource(r.Title, u.String(), string(text), true))
		counts[r.Kind]++
		if len(out) == 6 {
			break
		}
	}
	return out, nil
}
