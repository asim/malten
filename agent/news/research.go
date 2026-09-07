package news

import "github.com/asim/malten/agent"

func Researcher() agent.Researcher {
	return agent.Researcher{Name: "news", Objective: "Establish what the available headlines say about the event in the question. Read retained context or refresh the headlines, then select relevant stories. Headlines alone are not full reporting: do not infer causes, article details or confirmation of a person's account. Preserve article links, freshness and uncertainty. If the event is absent, say it is not established by these sources.", Read: Read, Sources: Context}
}
