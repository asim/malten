package reminder

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/asim/malten/agent"
)

// Search retrieves original texts, without requesting a second AI answer.
func Search(ctx context.Context, query string) ([]agent.Source, error) {
	body, _ := json.Marshal(map[string]any{"q": query, "summarise": false})
	raw, err := agent.ReadJSON(ctx, "POST", "https://reminder.dev/api/search", string(body))
	if err != nil {
		return nil, err
	}
	var data struct {
		References []struct {
			Text     string
			Metadata map[string]string
		}
	}
	if json.Unmarshal(raw, &data) != nil || data.References == nil {
		return nil, errors.New("invalid reminder search")
	}
	out := []agent.Source{}
	size := 0
	for _, r := range data.References {
		if strings.TrimSpace(r.Text) == "" || len([]rune(r.Text)) > 4000 {
			continue
		}
		m := r.Metadata
		var title, address string
		switch m["source"] {
		case "quran":
			if m["chapter"] == "" || m["verse"] == "" {
				continue
			}
			title = "Quran " + m["chapter"] + ":" + m["verse"]
			address = "https://reminder.dev/quran/" + url.PathEscape(m["chapter"]) + "#" + url.PathEscape(m["verse"])
		case "bukhari":
			title = "Sahih Bukhari · " + m["book"] + " · " + strings.TrimSpace(m["info"])
			address = "https://reminder.dev/hadith"
		case "names":
			title = m["english"] + " · " + m["meaning"]
			address = "https://reminder.dev/names"
		default:
			continue
		}
		if size+len(r.Text) > 12000 {
			continue
		}
		size += len(r.Text)
		out = append(out, agent.NewSource(title, address, r.Text, false))
		if len(out) == 6 {
			break
		}
	}
	return out, nil
}
