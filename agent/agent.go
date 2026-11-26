package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/asim/malten/server"
)

// Agent sends stream mentions and individual readers' requests to Malten on Mu.
type Agent struct {
	server   *server.Server
	client   *http.Client
	endpoint string
	token    string
}

func Run(ctx context.Context) *Agent {
	a := &Agent{
		server:   server.Default,
		client:   &http.Client{Timeout: 90 * time.Second},
		endpoint: muMaltenURL,
		token:    os.Getenv("MU_TOKEN"),
	}
	if a.token != "" {
		a.server.Commands = make(chan *server.Message, 32)
		go a.runMu(ctx)
	}
	go a.runReminders(ctx)
	return a
}

func (a *Agent) runMu(ctx context.Context) {
	s := a.server
	// Each stream has one active request. Waiting commands keep arrival order.
	pending := make(map[string][]*server.Message)
	completed := make(chan string)
	outstanding := 0
	start := func(command *server.Message) {
		go func() {
			defer func() {
				command.FinishCommand()
				select {
				case completed <- command.Stream:
				case <-ctx.Done():
				}
			}()
			if len(s.Retrieve(command.Id, command.Stream, 0, 0, 1)) == 0 {
				return
			}
			reply, err := a.ask(ctx, streamRequest(s, command))
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				log.Printf("Mu agent: %v", err)
				reply = muReply{Text: "Malten couldn’t get a reply from Mu. Please try again."}
			}
			s.Reply(command, reply.Text, a.endpoint, reply.Thread)
		}()
	}
	for {
		input := s.Commands
		if outstanding >= max(1, cap(s.Commands)) {
			input = nil
		}
		select {
		case <-ctx.Done():
			return
		case command := <-input:
			outstanding++
			waiting, active := pending[command.Stream]
			if active {
				pending[command.Stream] = append(waiting, command)
			} else {
				pending[command.Stream] = nil
				start(command)
			}
		case stream := <-completed:
			outstanding--
			waiting := pending[stream]
			if len(waiting) == 0 {
				delete(pending, stream)
				continue
			}
			pending[stream] = waiting[1:]
			start(waiting[0])
		}
	}
}

type contextMessage struct {
	Text   string `json:"text"`
	Author string `json:"author,omitempty"`
}

type muRequest struct {
	Prompt  string `json:"prompt"`
	Session string `json:"session,omitempty"`
	Thread  string `json:"thread,omitempty"`
	Context struct {
		Messages []contextMessage `json:"messages"`
	} `json:"context"`
}

const muMaltenURL = "https://micro.mu/agent/malten"
const muRequestBytes = 32_000

const reflectionGuidance = "Start with what the person shared. Use everyday language and follow their lead on faith. Help them put a thought into words or offer a question they can consider. When asked about Islam, explain unfamiliar terms, cite sources you can verify and distinguish the teaching from your own reflection. Refer personal religious rulings to a qualified scholar."

func excerpt(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	end := limit - len("…")
	for !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end] + "…"
}

// Include the specific post being discussed before filling the recent-history
// budget. Even the most heavily JSON-escaped text must fit Mu's request limit.
func (req *muRequest) addQuote(text, source string) {
	base, _ := json.Marshal(req)
	limit := min(1800, (muRequestBytes-len(base)-64)/6)
	if limit < len("…") {
		return
	}
	text = excerpt(text, 1400)
	if source != "" {
		text += "\n" + excerpt(source, 300)
	}
	req.Context.Messages = append(req.Context.Messages, contextMessage{Author: "Quoted reflection", Text: excerpt(text, limit)})
}

func streamRequest(s *server.Server, command *server.Message) muRequest {
	req := muRequest{Prompt: command.Text, Thread: s.Thread(command.Stream)}
	info := contextMessage{Author: "Malten", Text: "Malten is a shared network for reflection rooted in Islamic values and open to everyone. Posts expire after 24 hours by default. The home stream shares Malten’s daily reflection. The #reminder stream carries the original Qur’an, hadith and Name of Allah readings from Reminder.dev. Everyone in this stream can read the answer to an @malten mention. " + reflectionGuidance}
	req.Context.Messages = []contextMessage{info}
	haveReminder := false
	if command.Parent != "" {
		if parent := s.Retrieve(command.Parent, command.Stream, 0, 0, 1); len(parent) == 1 {
			quoted := parent[0]
			req.addQuote(quoted.Text, quoted.Source)
			req.addReminderSources(quoted)
			haveReminder = isDailyPost(quoted)
		}
	}
	// Account for JSON escaping in Mu's 32 KB HTTP body limit.
	base, _ := json.Marshal(req)
	remaining := muRequestBytes - len(base)
	contextRemaining := 16_000
	for _, m := range req.Context.Messages {
		contextRemaining -= len(m.Text) + len(m.Author)
	}
	var recent []contextMessage
	count := 0
	for _, m := range s.Retrieve("", command.Stream, -1, command.Created, server.MaxMessages) {
		if m.Id == command.Parent || strings.TrimSpace(m.Text) == "" {
			continue
		}
		entry := contextMessage{Text: m.Text}
		if isDailyPost(m) && m.Source != "" && !haveReminder {
			entry.Author = dailyByline(m)
			entry.Text = excerpt(m.Text, 1200) + "\n" + m.Source
			if sources := reminderContext(m); sources != "" {
				entry.Text += "\n" + sources
			}
			haveReminder = true
		} else if m.Type == "message" && m.Source == "" && count < 6 {
			count++
			entry.Text = excerpt(entry.Text, 1200)
		} else {
			continue
		}
		// Six bounded excerpts plus a reminder and app description fit the
		// 16,000-byte context limit. Skip an entry if its wire encoding won't fit.
		size := len(entry.Text) + len(entry.Author)
		if size > 5000 || size > contextRemaining {
			continue
		}
		encoded, _ := json.Marshal(entry)
		if len(encoded)+1 > remaining {
			continue
		}
		remaining -= len(encoded) + 1
		contextRemaining -= size
		recent = append(recent, entry)
	}
	slices.Reverse(recent)
	req.Context.Messages = append(req.Context.Messages, recent...)
	return req
}

type muReply struct {
	Text   string `json:"text"`
	Thread string `json:"thread"`
}

type muStatusError int

func (status muStatusError) Error() string { return fmt.Sprintf("HTTP %d", int(status)) }

func (a *Agent) ask(ctx context.Context, request muRequest) (muReply, error) {
	var reply muReply
	payload, err := json.Marshal(request)
	if err != nil {
		return reply, err
	}
	if len(payload) > muRequestBytes {
		return reply, muStatusError(http.StatusRequestEntityTooLarge)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoint, bytes.NewReader(payload))
	if err != nil {
		return reply, err
	}
	req.Header.Set("Content-Type", "application/json")
	if err := a.do(req, &reply); err != nil {
		return reply, err
	}
	if strings.TrimSpace(reply.Text) == "" {
		return reply, errors.New("empty reply")
	}
	return reply, nil
}

func (a *Agent) do(req *http.Request, output any) error {
	req.Header.Set("Authorization", "Bearer "+a.token)
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return muStatusError(resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 512<<10)).Decode(output)
}
