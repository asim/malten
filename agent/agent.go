// Package agent contains server-owned loops with specific objectives.
package agent

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"
	_ "time/tzdata"
)

// Stream describes an agent's public stream for discovery.
type Stream struct {
	Tag string
}

// PublishPhoto sends agent text and optional photos through moderation.
type PublishPhoto func(context.Context, string, string, string, string, ...string) error

// Cities provide the fixed locations used by Nature.

type City struct{ Tag, Timezone string }

var Cities = []City{
	{"london", "Europe/London"}, {"paris", "Europe/Paris"},
	{"nyc", "America/New_York"}, {"sf", "America/Los_Angeles"},
	{"dubai", "Asia/Dubai"}, {"singapore", "Asia/Singapore"},
}

// Observation contains only approved public text or an anonymous moderation event.
type Observation struct {
	Photo                  string `json:"-"`
	ID, Stream, Text, Kind string
	At                     time.Time
}
type Action struct{ Stream, Text, Place string }
type Decision struct {
	Summary  string
	Action   *Action
	Evidence []string
}
type View struct {
	Now               time.Time
	Objective         string
	Name              string
	SourceUnavailable bool
	Records           []Record
	Observations      []Observation
}
type Agent struct {
	DisplayName     string
	Name, Objective string
	Read            func(context.Context, time.Time) (json.RawMessage, error)
	Decide          func(context.Context, View) (Decision, error)
	// Check constrains actions beyond the common destination and length checks.
	Check func(View, Decision) bool
	Media func(Action) (string, string)
}
type Loop struct {
	Agent   Agent
	Memory  *Memory
	Observe func() []Observation
	Publish PublishPhoto
}

// Run is the common lifecycle: read, record, build context, decide, act.
// Source errors do not erase context. A failed action retries with the same key.
func (l Loop) Run(ctx context.Context) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if ctx.Err() != nil {
				return
			}
			err := l.Step(ctx, time.Now())
			l.Memory.RecordCycle(l.Agent.Name, err)
			if err != nil && ctx.Err() == nil {
				log.Printf("%s: cycle failed: %v", l.Agent.Name, err)
			}
			timer.Reset(10 * time.Minute)
		}
	}
}
func (l Loop) Step(ctx context.Context, now time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := l.Memory.Expire(now); err != nil {
		return err
	}
	history := l.Memory.Read(l.Agent.Name, now)
	for _, r := range history {
		if r.Status == "pending" {
			// A delayed update stops being "now". Never publish yesterday's weather
			// or reply to an observation that has since disappeared.
			expired := now.Sub(r.At) >= time.Hour
			if r.Action.Stream != l.Agent.Name && l.Agent.Name != "reminder" {
				present := false
				for _, o := range l.Observe() {
					if o.Stream == r.Action.Stream && o.Kind == "human" && now.Sub(o.At) < time.Hour {
						present = true
					}
				}
				expired = expired || !present
			}
			if expired {
				r.Status = "expired"
				if err := l.Memory.Write(l.Agent.Name, r); err != nil {
					return err
				}
				continue
			}
			return l.act(ctx, r)
		}
	}
	data, err := l.Agent.Read(ctx, now)
	sourceUnavailable := err != nil
	if err != nil {
		for i := len(history) - 1; i >= 0; i-- {
			if history[i].Kind == "source" {
				data = history[i].Data
				break
			}
		}
		if len(data) == 0 {
			return err
		}
	}
	if !json.Valid(data) || len(data) > 128<<10 {
		return errors.New("invalid source data")
	}
	sourceID := Key(data)
	exists := false
	for _, r := range history {
		if r.ID == sourceID {
			exists = true
		}
	}
	if !exists {
		if err = l.Memory.Write(l.Agent.Name, Record{ID: sourceID, At: now, Kind: "source", Data: data}); err != nil {
			return err
		}
	}
	history = l.Memory.Read(l.Agent.Name, now)
	observations := l.Observe()
	// Fingerprint inputs only, not our own summaries: no self-triggering loop.
	var events []string
	for _, r := range history {
		if r.Kind == "moderation" {
			events = append(events, r.ID)
		}
	}
	input, _ := json.Marshal(struct {
		Source       string
		Objective    string
		Events       []string
		Observations []Observation
	}{sourceID, l.Agent.Objective, events, observations})
	id := "decision-" + Key(input)
	for _, r := range history {
		if r.ID == id {
			return nil
		}
	}
	view := View{SourceUnavailable: sourceUnavailable, Now: now, Objective: l.Agent.Objective, Name: l.Agent.Name, Records: history, Observations: observations}
	decide := l.Agent.Decide
	if decide == nil {
		decide = Decide
	}
	decision, err := decide(ctx, view)
	if err != nil {
		return err
	}
	if len([]rune(decision.Summary)) > 3000 {
		return errors.New("summary too long")
	}
	record := Record{ID: id, At: now, Kind: "decision", Summary: decision.Summary, Status: "done"}
	if decision.Action != nil {
		a := decision.Action
		if !validAction(l.Agent.Name, view, decision) || (l.Agent.Check != nil && !l.Agent.Check(view, decision)) {
			return errors.New("unsupported action")
		}
		// Limit publication across all destinations, including restarts.
		for _, r := range history {
			if r.Status == "sent" && r.Action != nil && now.Sub(r.At) < time.Hour {
				return l.Memory.Write(l.Agent.Name, record)
			}
		}
		record.Action = a
		record.Status = "pending"
	}
	if err = l.Memory.Write(l.Agent.Name, record); err != nil {
		return err
	}
	if record.Action != nil {
		return l.act(ctx, record)
	}
	return nil
}
func validAction(name string, v View, d Decision) bool {
	a := d.Action
	if a.Stream == "" || strings.TrimSpace(a.Text) == "" || len([]rune(a.Text)) > 1200 || len(d.Evidence) == 0 {
		return false
	}
	evidence := false
	for _, id := range d.Evidence {
		found := false
		for _, r := range v.Records {
			if (r.Kind == "source" || r.Kind == "moderation") && r.ID == id {
				found = true
			}
		}
		for _, o := range v.Observations {
			if o.ID == id {
				found = true
			}
		}
		if !found {
			return false
		}
		evidence = true
	}
	if !evidence {
		return false
	}
	if a.Stream == name {
		return true
	}
	if name == "reminder" {
		for _, r := range v.Records {
			var event struct{ Stream string }
			if r.Kind == "moderation" && json.Unmarshal(r.Data, &event) == nil && event.Stream == a.Stream {
				return true
			}
		}
		return false
	}
	// A public destination must have a recent human observation; no blind seeding.
	if name == "news" {
		return false
	}
	for _, o := range v.Observations {
		if o.Stream == a.Stream && o.Kind == "human" && v.Now.Sub(o.At) < time.Hour {
			for _, id := range d.Evidence {
				if id == o.ID {
					return true
				}
			}
		}
	}
	return false
}
func (l Loop) act(ctx context.Context, r Record) error {
	text, photo := r.Action.Text, ""
	if l.Agent.Media != nil {
		var caption string
		photo, caption = l.Agent.Media(*r.Action)
		if caption != "" {
			text += "\n\n" + caption
		}
	}
	name := l.Agent.DisplayName
	if name == "" {
		name = Label(l.Agent.Name) + " · AI"
	}
	err := l.Publish(ctx, r.Action.Stream, text, name, photo, r.ID)
	if err != nil {
		if err.Error() != "capture not suitable for sharing" && err.Error() != "invalid capture" {
			return err
		}
		r.Status = "blocked"
	} else {
		r.Status = "sent"
	}
	return l.Memory.Write(l.Agent.Name, r)
}

func Label(tag string) string {
	switch tag {
	case "uk":
		return "UK"
	case "us":
		return "US"
	case "mena":
		return "Middle East"
	case "nyc":
		return "New York"
	case "sf":
		return "San Francisco"
	}
	if tag == "" {
		return ""
	}
	return strings.ToUpper(tag[:1]) + tag[1:]
}

// IsUnlisted recognises short generated names and older random identifiers.
func IsUnlisted(tag string) bool {
	if len(tag) == 10 {
		digit := false
		for _, r := range tag {
			if r >= '0' && r <= '9' {
				digit = true
			} else if r < 'a' || r > 'z' {
				return false
			}
		}
		return digit
	}
	if len(tag) != 32 {
		return false
	}
	_, err := hex.DecodeString(tag)
	return err == nil
}
