package llm

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"sync/atomic"
)

// Stub is a deterministic, rule-based implementation of LLM. It lets the entire
// agent, tool, policy and persistence machinery run and be tested without an
// API key or network access. It is intentionally simple: it reads the latest
// customer intent, sees which tools have already run this turn (from the
// transcript), and emits the next sensible tool call or a final message.
//
// It is not meant to be a good model — it is a stand-in that makes the *system*
// exercisable and the evaluation reproducible. Swap in ClaudeLLM for real use.
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

var (
	reOrder  = regexp.MustCompile(`(?i)ORD-\d+`)
	reAmount = regexp.MustCompile(`\$\s*(\d+(?:\.\d{1,2})?)`)
	reCustID = regexp.MustCompile(`customer_id is ([A-Za-z0-9\-]+)`)
)

// Complete implements the rule-based turn.
func (s *Stub) Complete(ctx context.Context, req Request) (*Response, error) {
	raw := lastUserText(req.Messages)
	intent := strings.ToLower(raw)
	custID := parseCustomerID(req.System)
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
		return say("Hi, I'm Malten. How can I help you today?"), nil

	// Explicit request for a human.
	case containsAny(intent, "speak to a human", "talk to a human", "human agent", "real person", "representative", "speak to someone", "talk to a person"):
		return emit("escalate_to_human", map[string]any{"reason": "customer asked to speak with a human"}), nil

	// Refund flow.
	case containsAny(intent, "refund", "money back", "reimburse", "charge back"):
		if custID == "" {
			return say("I can help with a refund. Could you share your customer id so I can look up the order?"), nil
		}
		acct, haveAcct := lastAccount(req.Messages)
		if !haveAcct {
			return emit("account_lookup", map[string]any{"customer_id": custID}), nil
		}
		if res, ran := done["issue_refund"]; ran {
			if isErrorResult(req.Messages, "issue_refund") {
				return say("I'm sorry, I wasn't able to process that refund: " + strings.TrimPrefix(res, "action not permitted: ") + " Is there anything else I can help with?"), nil
			}
			return say(res + " The refund should appear on your statement within a few business days. Anything else?"), nil
		}
		orderID, amount := pickOrder(raw, acct)
		return emit("issue_refund", map[string]any{"order_id": orderID, "amount": amount}), nil

	// Password reset flow.
	case containsAny(intent, "reset my password", "reset password", "forgot my password", "can't log in", "cant log in", "cannot log in", "locked out", "log back in"):
		if custID == "" {
			return say("I can send a password reset link. What's your customer id?"), nil
		}
		if res, ran := done["reset_password"]; ran {
			if isErrorResult(req.Messages, "reset_password") {
				return say("I couldn't reset the password: " + strings.TrimPrefix(res, "action not permitted: ")), nil
			}
			return say(res + " Check your inbox and follow the link to set a new password."), nil
		}
		return emit("reset_password", map[string]any{"customer_id": custID}), nil

	// Ticket flow (bugs, complaints, feature requests).
	case containsAny(intent, "bug", "broken", "not working", "doesn't work", "crash", "error", "complaint", "feature request", "problem with", "glitch"):
		if res, ran := done["create_ticket"]; ran {
			return say(res + " Our team will follow up. Is there anything else I can help with?"), nil
		}
		return emit("create_ticket", map[string]any{"summary": summarize(raw), "priority": pickPriority(intent)}), nil

	// Knowledge / how-to questions.
	case looksLikeQuestion(intent) || containsAny(intent, "how do", "how can", "where is", "what is", "export", "cancel", "pricing", "plan", "rate limit", "api"):
		if res, ran := done["kb_search"]; ran {
			return say(answerFromKB(res)), nil
		}
		return emit("kb_search", map[string]any{"query": raw, "k": 3}), nil

	default:
		if _, ran := done["kb_search"]; !ran {
			return emit("kb_search", map[string]any{"query": raw, "k": 3}), nil
		}
		if res, ran := done["kb_search"]; ran {
			return say(answerFromKB(res)), nil
		}
		return say("I'm not sure I can help with that directly, but I can look into it. Could you tell me a bit more?"), nil
	}
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
// a result after the most recent customer text message (i.e. this turn).
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

// isErrorResult reports whether the latest result for a tool this turn was an error.
func isErrorResult(msgs []Message, tool string) bool {
	names := idToName(msgs)
	start := lastUserTextIndex(msgs)
	var isErr bool
	for i := start; i < len(msgs); i++ {
		for _, b := range msgs[i].Content {
			if b.Type == BlockToolResult && names[b.ToolUseID] == tool {
				isErr = b.IsError
			}
		}
	}
	return isErr
}

type stubAccount struct {
	CustomerID string `json:"customer_id"`
	Orders     []struct {
		OrderID  string  `json:"order_id"`
		Amount   float64 `json:"amount"`
		Refunded bool    `json:"refunded"`
	} `json:"orders"`
}

// lastAccount returns the most recent successful account_lookup result parsed
// from anywhere in the transcript (so multi-turn context is available).
func lastAccount(msgs []Message) (stubAccount, bool) {
	names := idToName(msgs)
	var acct stubAccount
	var found bool
	for _, m := range msgs {
		for _, b := range m.Content {
			if b.Type == BlockToolResult && names[b.ToolUseID] == "account_lookup" && !b.IsError {
				var a stubAccount
				if json.Unmarshal([]byte(b.Content), &a) == nil {
					acct = a
					found = true
				}
			}
		}
	}
	return acct, found
}

// --- intent helpers ---------------------------------------------------------

func parseCustomerID(system string) string {
	m := reCustID.FindStringSubmatch(system)
	if len(m) == 2 {
		return m[1]
	}
	return ""
}

// pickOrder chooses which order and amount to refund from the message and the
// account data. An explicit ORD id in the message always wins (even if it is
// not the customer's — the policy layer will catch that).
func pickOrder(raw string, acct stubAccount) (string, float64) {
	low := strings.ToLower(raw)
	if m := reOrder.FindString(raw); m != "" {
		id := strings.ToUpper(m)
		amount := 1.0
		for _, o := range acct.Orders {
			if strings.EqualFold(o.OrderID, id) {
				amount = o.Amount
			}
		}
		if am := parseAmount(raw); am > 0 {
			amount = am
		}
		return id, amount
	}
	// Positional references resolved against the account's orders.
	idx := 0
	if containsAny(low, "second", "2nd", "other", "annual", "upgrade") && len(acct.Orders) > 1 {
		idx = 1
	}
	// Otherwise prefer the first order that has not been refunded.
	if idx == 0 {
		for i, o := range acct.Orders {
			if !o.Refunded {
				idx = i
				break
			}
		}
	}
	if idx >= len(acct.Orders) {
		idx = 0
	}
	if len(acct.Orders) == 0 {
		return "", 0
	}
	o := acct.Orders[idx]
	amount := o.Amount
	if am := parseAmount(raw); am > 0 {
		amount = am
	}
	return o.OrderID, amount
}

func parseAmount(raw string) float64 {
	m := reAmount.FindStringSubmatch(raw)
	if len(m) == 2 {
		return atof(m[1])
	}
	return 0
}

func atof(s string) float64 {
	var whole, frac float64
	var div float64 = 1
	dot := false
	for _, r := range s {
		if r == '.' {
			dot = true
			continue
		}
		if r < '0' || r > '9' {
			continue
		}
		d := float64(r - '0')
		if dot {
			div *= 10
			frac = frac*10 + d
		} else {
			whole = whole*10 + d
		}
	}
	return whole + frac/div
}

func pickPriority(intent string) string {
	switch {
	case containsAny(intent, "urgent", "asap", "immediately", "critical", "down", "outage"):
		return "urgent"
	case containsAny(intent, "important", "high", "blocking", "can't work", "cant work"):
		return "high"
	default:
		return "normal"
	}
}

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
	for _, w := range []string{"how ", "what ", "why ", "where ", "when ", "which ", "does ", "do i ", "can i ", "is there"} {
		if strings.HasPrefix(intent, strings.TrimSpace(w)) || strings.Contains(intent, w) {
			return true
		}
	}
	return false
}

// answerFromKB turns a kb_search result into a short friendly answer.
func answerFromKB(res string) string {
	lines := strings.Split(res, "\n")
	// The first result is "1. Title" followed by its content line.
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
	return body + "\n\nLet me know if that helps or if you'd like more detail."
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
