package nature

import (
	"encoding/json"
	"time"

	"github.com/asim/malten/agent"
)

func Context(record agent.Record) []agent.Source {
	var data struct {
		Places []struct {
			Name, Timezone string
			Data           json.RawMessage
		}
	}
	if json.Unmarshal(record.Data, &data) != nil {
		return nil
	}
	var out []agent.Source
	for _, p := range data.Places {
		if p.Name == "" || len(p.Data) > 2500 {
			continue
		}
		text := "Weather model estimates, not live observations. " + p.Name + " · " + p.Timezone + ". Retrieved " + record.At.UTC().Format(time.RFC3339) + "\n" + string(p.Data)
		out = append(out, agent.NewSource(p.Name+" · Open-Meteo", "https://open-meteo.com/", text, false))
		if len(out) == 6 {
			break
		}
	}
	return out
}
