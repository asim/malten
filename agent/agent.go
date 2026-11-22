package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/asim/malten/server"
)

func Run(ctx context.Context) {
	if token := os.Getenv("MU_TOKEN"); token != "" {
		server.Default.Commands = make(chan *server.Message, 32)
		client := &http.Client{Timeout: 90 * time.Second}
		go runMu(ctx, server.Default, client, "https://micro.mu/agent/malten", token)
	}
	go runReminders(ctx, server.Default.Events)
}

func runMu(ctx context.Context, s *server.Server, client *http.Client, endpoint, token string) {
	// Each stream has one active request. Waiting commands keep arrival order.
	pending := make(map[string][]*server.Message)
	completed := make(chan string)
	outstanding := 0
	start := func(command *server.Message) {
		go func() {
			reply, err := promptMu(ctx, client, endpoint, token, streamPrompt(s, command), s.Thread(command.Stream))
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

// Mu accepts a prompt and a continuation thread. Supply the app context on
// every turn since a visitor may join an existing shared conversation.
func streamPrompt(s *server.Server, command *server.Message) string {
	const intro = "You are Malten on malten.me, a network for reflection. People share thoughts in a stream that expires after 24 hours by default. The home stream also has a daily Qur’an passage from Reminder.dev. You answer when someone mentions @malten, and everyone in that stream can read the answer. If someone asks what this is, introduce the space briefly. When asked about a passage, use its reference. Stay focused on the shared conversation; do not offer or access anyone’s private mail, notes, calendar, or account data."
	context := intro
	question := "\n\nVisitor’s question:\n" + command.Text
	if command.Stream == server.DefaultStream {
		messages := s.Retrieve("", command.Stream, 1, 0, server.MaxMessages)
		for i := len(messages) - 1; i >= 0; i-- {
			m := messages[i]
			if m.Created > command.Created || !strings.HasPrefix(m.Id, "reminder-") || m.Source == "" {
				continue
			}
			passage := "\n\nDaily Reminder (" + m.Source + "):\n" + m.Text
			// Mu accepts 8,000 bytes per prompt. Keep the visitor's words intact.
			if len(context)+len(passage)+len(question) <= 8000 {
				context += passage
			} else {
				context += "\n\nDaily Reminder: " + m.Source
			}
			break
		}
	}
	return context + question
}

type muReply struct {
	Text   string `json:"text"`
	Thread string `json:"thread"`
}

func promptMu(ctx context.Context, client *http.Client, endpoint, token, prompt, thread string) (muReply, error) {
	var reply muReply
	payload, err := json.Marshal(struct {
		Prompt string `json:"prompt"`
		Thread string `json:"thread,omitempty"`
	}{prompt, thread})
	if err != nil {
		return reply, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return reply, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return reply, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return reply, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&reply); err != nil {
		return reply, err
	}
	if strings.TrimSpace(reply.Text) == "" {
		return reply, errors.New("empty reply")
	}
	return reply, nil
}
