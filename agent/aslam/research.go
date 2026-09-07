package aslam

import (
	"encoding/json"

	"github.com/asim/malten/agent"
)

func Researcher() agent.Researcher {
	return agent.Researcher{Name: "aslam", Objective: "Investigate Islamic understanding surrounding the question, including gratitude, patience, purpose and responsibility. Search relevant indexed knowledge when retained context is insufficient. Distinguish Quran and hadith from scholarly explanation, and explain where short excerpts cannot settle a question. Do not turn reflection into a ruling or imply that a retrieved opinion is universal consensus.", Read: Read, Sources: Context, Search: Search}
}

func Context(record agent.Record) []agent.Source {
	var data struct{ Data json.RawMessage }
	if json.Unmarshal(record.Data, &data) != nil {
		return nil
	}
	sources, _ := searchResults(data.Data)
	return sources
}
