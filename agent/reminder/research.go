package reminder

import (
	"encoding/json"
	"net/url"

	"github.com/asim/malten/agent"
)

func Researcher() agent.Researcher {
	return agent.Researcher{Name: "reminder", Objective: "Find relevant Quran, hadith and names of Allah. Return primary texts with their references and explain their relevance carefully. Prefer targeted search over an unrelated latest passage. Keep generated daily reflections separate from source material; do not use them as religious evidence.", Read: Read, Sources: Context, Search: Search}
}

func Context(record agent.Record) []agent.Source {
	var data struct {
		Data struct {
			Verse, Hadith, Name string
			Links               map[string]string
		}
	}
	if json.Unmarshal(record.Data, &data) != nil {
		return nil
	}
	base, _ := url.Parse("https://reminder.dev")
	var out []agent.Source
	for _, item := range []struct{ key, title, text string }{{"verse", "Quran", data.Data.Verse}, {"hadith", "Hadith", data.Data.Hadith}, {"name", "Names of Allah", data.Data.Name}} {
		if item.text == "" || len(item.text) > 6000 {
			continue
		}
		u, err := url.Parse(data.Data.Links[item.key])
		if err != nil {
			continue
		}
		u = base.ResolveReference(u)
		if u.Scheme != "https" || u.Host != "reminder.dev" || u.User != nil {
			continue
		}
		out = append(out, agent.NewSource(item.title, u.String(), item.text, false))
	}
	return out
}
