package llm

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"sync/atomic"
)

// reIssueID pulls the first "[ISS-...]" id out of the injected issue context.
var reIssueID = regexp.MustCompile(`\[(ISS-[A-Za-z0-9]+)\]`)

func firstIssueID(system string) string {
	if m := reIssueID.FindStringSubmatch(system); len(m) == 2 {
		return m[1]
	}
	return ""
}

// Stub is a deterministic, rule-based implementation of LLM. It lets the entire
// agent, tool, policy and persistence machinery run and be tested without an
// API key or network access. It reads the latest user message, sees which tools
// have already run this turn (from the transcript), and emits the next sensible
// tool call or a final message.
//
// It is not meant to be a good model — it is a stand-in that makes the *system*
// exercisable and the evaluation reproducible. Swap in Claude for real use.
type Stub struct{}

// NewStub returns a Stub.
func NewStub() *Stub { return &Stub{} }

// Name identifies the backend.
func (s *Stub) Name() string { return "stub" }

var stubCounter uint64

func stubID() string {
	n := atomic.AddUint64(&stubCounter, 1)
	return "toolu_stub_" + itoa(n)
}

func itoa(n uint64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// Complete implements the rule-based turn.
func (s *Stub) Complete(ctx context.Context, req Request) (*Response, error) {
	raw := lastUserText(req.Messages)
	intent := strings.ToLower(raw)
	done := doneSince(req.Messages)

	emit := func(name string, input any) *Response {
		b, _ := json.Marshal(input)
		return &Response{StopReason: StopToolUse, Content: []Block{ToolUse(stubID(), name, b)}}
	}
	say := func(text string) *Response {
		return &Response{StopReason: StopEndTurn, Content: []Block{Text(text)}}
	}

	switch {
	case strings.TrimSpace(intent) == "":
		return say("Hi, I'm Malten. I'm here to listen. What's on your mind?"), nil

	// Crisis: respond with care, take it seriously, signpost help. No tools.
	case containsAny(intent, "kill myself", "suicide", "suicidal", "end my life", "want to die",
		"don't want to be alive", "dont want to be alive", "harm myself", "hurt myself",
		"self-harm", "self harm", "better off dead"):
		return say("I'm really glad you told me, and I'm so sorry you're feeling this much pain. " +
			"You deserve support right now, and I don't want you to be alone with this. " +
			"Please reach out to a crisis line or emergency services straight away — in the UK you can call Samaritans free on 116 123, any time, or your local emergency number. " +
			"I'm here with you, and I'd like to keep talking."), nil

	// Made progress on something -> mark the relevant issue done.
	case containsAny(intent, "mark it done", "mark that done", "mark it as done", "i did it",
		"i've done it", "i have done it", "finished it", "i finished", "completed it",
		"sorted it", "resolved it", "closed it", "done with that", "ticked it off"):
		iss := firstIssueID(req.System)
		if iss == "" {
			return say("That's brilliant — well done for getting it done. I don't see it in your open issues, but I'm really glad."), nil
		}
		if _, ran := done["update_issue"]; ran {
			return say("Nice — that's a real step. I've marked it as done. How did it feel to get it off your plate?"), nil
		}
		return emit("update_issue", map[string]any{"id": iss, "status": "closed"}), nil

	// Something to keep working on / plan out -> log an issue.
	case containsAny(intent, "make a plan", "need a plan", "help me plan", "come up with a plan",
		"a plan for", "keep working on", "work on this", "work through this", "track this",
		"procrastinat", "put it on my list"):
		if _, ran := done["create_issue"]; ran {
			return say("Okay — I've logged that as an issue you can come back to, and we can build out the plan together whenever you're ready. What feels like the smallest first step?"), nil
		}
		return emit("create_issue", map[string]any{
			"title": summarize(raw),
			"plan":  "Start with one small, doable step.",
		}), nil

	// Looking for a technique / feeling anxious, overwhelmed, etc. -> search.
	case looksLikeQuestion(intent) || containsAny(intent, "anxious", "anxiety", "panic",
		"calm down", "calm me", "breathe", "breathing", "grounding", "ground me",
		"overwhelmed", "can't sleep", "cant sleep", "stressed", "racing thoughts"):
		if res, ran := done["search"]; ran {
			return say(answerFromKB(res)), nil
		}
		return emit("search", map[string]any{"query": raw, "k": 3}), nil

	default:
		return say("Thank you for telling me that. That sounds like a lot to sit with. Would it help to talk through what's weighing on you most right now?"), nil
	}
}

// Stream runs the same deterministic turn as Complete, then delivers any final
// text a word at a time so the streaming path is exercised without a network.
func (s *Stub) Stream(ctx context.Context, req Request, onText func(string)) (*Response, error) {
	resp, err := s.Complete(ctx, req)
	if err != nil {
		return nil, err
	}
	if onText != nil {
		for _, b := range resp.Content {
			if b.Type == BlockText && b.Text != "" {
				for _, chunk := range strings.SplitAfter(b.Text, " ") {
					if chunk != "" {
						onText(chunk)
					}
				}
			}
		}
	}
	return resp, nil
}

// --- history inspection helpers ---------------------------------------------

func lastUserText(msgs []Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != RoleUser {
			continue
		}
		for _, b := range msgs[i].Content {
			if b.Type == BlockText && strings.TrimSpace(b.Text) != "" {
				return b.Text
			}
		}
	}
	return ""
}

func lastUserTextIndex(msgs []Message) int {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != RoleUser {
			continue
		}
		for _, b := range msgs[i].Content {
			if b.Type == BlockText && strings.TrimSpace(b.Text) != "" {
				return i
			}
		}
	}
	return 0
}

// idToName maps tool_use ids to tool names across the whole transcript.
func idToName(msgs []Message) map[string]string {
	m := map[string]string{}
	for _, msg := range msgs {
		for _, b := range msg.Content {
			if b.Type == BlockToolUse {
				m[b.ID] = b.Name
			}
		}
	}
	return m
}

// doneSince returns tool name -> latest result content for tools that produced
// a result after the most recent user text message (i.e. this turn).
func doneSince(msgs []Message) map[string]string {
	names := idToName(msgs)
	start := lastUserTextIndex(msgs)
	out := map[string]string{}
	for i := start; i < len(msgs); i++ {
		for _, b := range msgs[i].Content {
			if b.Type == BlockToolResult {
				if n, ok := names[b.ToolUseID]; ok {
					out[n] = b.Content
				}
			}
		}
	}
	return out
}

// --- intent helpers ---------------------------------------------------------

func summarize(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) > 120 {
		return raw[:117] + "..."
	}
	return raw
}

func looksLikeQuestion(intent string) bool {
	if strings.Contains(intent, "?") {
		return true
	}
	for _, w := range []string{"how ", "what ", "why ", "where ", "when ", "which ", "can i ", "should i ", "is there"} {
		if strings.HasPrefix(intent, strings.TrimSpace(w)) || strings.Contains(intent, w) {
			return true
		}
	}
	return false
}

// answerFromKB turns a search result into a short, warm answer.
func answerFromKB(res string) string {
	lines := strings.Split(res, "\n")
	var body string
	for i, ln := range lines {
		if strings.HasPrefix(ln, "1. ") && i+1 < len(lines) {
			body = lines[i+1]
			break
		}
	}
	if body == "" {
		body = res
	}
	return "Here's something gentle you could try: " + body + "\n\nWould you like to try it together, or talk about what's bringing this on?"
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

var _ = lastUserTextIndex // used above; keep referenced for clarity
