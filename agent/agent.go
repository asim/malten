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

// Agent sends stream mentions and reader actions to their hosted agents on Mu.
type Agent struct {
	server  *server.Server
	client  *http.Client
	baseURL string
	token   string
}

func Run(ctx context.Context) *Agent {
	a := &Agent{
		server:  server.Default,
		client:  &http.Client{Timeout: 90 * time.Second},
		baseURL: "https://micro.mu/agent",
		token:   os.Getenv("MU_TOKEN"),
	}
	if a.token != "" {
		a.server.Commands = make(chan *server.Message, 32)
		go a.runMu(ctx)
	}
	go runReminders(ctx, a.server.Events)
	return a
}

func (a *Agent) runMu(ctx context.Context) {
	s := a.server
	endpoint := a.baseURL + "/malten"
	// Each stream has one active request. Waiting commands keep arrival order.
	pending := make(map[string][]*server.Message)
	completed := make(chan string)
	outstanding := 0
	start := func(command *server.Message) {
		go func() {
			reply, err := a.ask(ctx, endpoint, streamRequest(s, command))
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				log.Printf("Mu agent: %v", err)
				reply = muReply{Text: "Malten couldn’t get a reply from Mu. Please try again."}
			}
			s.Reply(command, reply.Text, endpoint, reply.Thread)
			select {
			case completed <- command.Stream:
			case <-ctx.Done():
			}
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
	Thread  string `json:"thread,omitempty"`
	Context struct {
		Messages []contextMessage `json:"messages"`
	} `json:"context"`
}

func streamRequest(s *server.Server, command *server.Message) muRequest {
	req := muRequest{Prompt: command.Text, Thread: s.Thread(command.Stream)}
	info := contextMessage{Author: "Malten", Text: "Malten is a shared network for reflection. Posts expire after 24 hours by default. The home stream includes a daily Qur’an passage. Everyone in this stream can read the answer to an @malten mention."}
	req.Context.Messages = []contextMessage{info}
	// Account for JSON escaping in Mu's 32 KB HTTP body limit.
	base, _ := json.Marshal(req)
	remaining := 32_000 - len(base)
	var recent []contextMessage
	count := 0
	haveReminder := false
	for _, m := range s.Retrieve("", command.Stream, -1, command.Created, server.MaxMessages) {
		if strings.TrimSpace(m.Text) == "" {
			continue
		}
		entry := contextMessage{Text: m.Text}
		if strings.HasPrefix(m.Id, "reminder-") && m.Source != "" && command.Stream == server.DefaultStream && !haveReminder {
			entry.Author = "Daily Reminder"
			entry.Text += "\n" + m.Source
			haveReminder = true
		} else if m.Type == "message" && m.Source == "" && count < 6 {
			count++
			if len(entry.Text) > 1200 {
				end := 1197
				for !utf8.RuneStart(entry.Text[end]) {
					end--
				}
				entry.Text = entry.Text[:end] + "…"
			}
		} else {
			continue
		}
		// Six bounded excerpts plus a reminder and app description fit the
		// 16,000-byte context limit. Skip an entry if its wire encoding won't fit.
		if len(entry.Text)+len(entry.Author) > 5000 {
			continue
		}
		encoded, _ := json.Marshal(entry)
		if len(encoded)+1 > remaining {
			continue
		}
		remaining -= len(encoded) + 1
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

func (a *Agent) ask(ctx context.Context, endpoint string, request muRequest) (muReply, error) {
	var reply muReply
	payload, err := json.Marshal(request)
	if err != nil {
		return reply, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return reply, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.token)
	resp, err := a.client.Do(req)
	if err != nil {
		return reply, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return reply, muStatusError(resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 512<<10)).Decode(&reply); err != nil {
		return reply, err
	}
	if strings.TrimSpace(reply.Text) == "" {
		return reply, errors.New("empty reply")
	}
	return reply, nil
}
