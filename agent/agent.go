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
	for {
		select {
		case <-ctx.Done():
			return
		case command := <-s.Commands:
			thread := s.Thread(command.Stream)
			reply, err := promptMu(ctx, client, endpoint, token, command.Text, thread)
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				log.Printf("Mu agent: %v", err)
				s.Reply(command, "Malten couldn’t get a reply from Mu. Please try again.", endpoint, "")
				continue
			}
			s.Reply(command, reply.Text, endpoint, reply.Thread)
		}
	}
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
