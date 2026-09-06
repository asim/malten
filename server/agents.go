package server

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/asim/malten/agent"
	"github.com/asim/malten/agent/reflection"
	"log"
	"time"
)

// UseAgentContext is called before serving. The fixed policy remains authoritative;
// source context supports review but cannot change the moderation contract.
func (s *Server) UseAgentContext(memory *agent.Memory) {
	s.summarise = func(ctx context.Context, captures []agent.Observation) (reflection.Result, error) {
		return reflection.Summarise(ctx, captures, memory)
	}
	s.stream.moderate = func(ctx context.Context, p Post) (bool, error) {
		records := memory.Read("reminder", time.Now())
		for i := len(records) - 1; i >= 0; i-- {
			if records[i].Kind == "source" {
				p.guidance = string(records[i].Data)
				if len(p.guidance) > 12000 {
					p.guidance = p.guidance[:12000]
				}
				break
			}
		}
		allowed, err := moderate(ctx, p)
		if err == nil && !allowed && p.Agent == "" {
			now := time.Now()
			data, _ := json.Marshal(struct{ Stream, Event string }{p.Stream, "confirmed moderation rejection"})
			id := agent.Key([]byte(fmt.Sprintf("%s:%d", p.Stream, now.UnixNano())))
			if e := memory.Write("reminder", agent.Record{ID: id, At: now, Kind: "moderation", Data: data}); e != nil {
				log.Printf("reminder: could not record moderation event: %v", e)
			}
		}
		return allowed, err
	}
}

// AgentObservations excludes private drafts, hidden content and identity.
// Referenced human observations stay authoritative in the public store: deletion
// and reporting remove them from subsequent cycles rather than copying them.
func (s *Server) AgentObservations() []agent.Observation {
	b := s.stream
	b.Lock()
	defer b.Unlock()
	b.prune(time.Now())
	var out []agent.Observation
	for _, p := range b.posts {
		if p.Agent == "" && !p.hidden && !agent.IsUnlisted(p.Stream) {
			out = append(out, agent.Observation{ID: p.ID, Stream: p.Stream, Text: p.Text, Photo: p.Photo, Kind: "human", At: time.UnixMilli(p.Created)})
		}
	}
	if len(out) > 40 {
		out = out[len(out)-40:]
	}
	return out
}
