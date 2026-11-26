package agent

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/asim/malten/server"
)

type readerTurn struct{ Question, Answer string }

// Ask answers one reader about a specific reflection. The shared stream thread
// is used only by public @malten mentions.
func (a *Agent) Ask(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if a.token == "" {
		http.Error(w, "Malten unavailable", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var input struct {
		Stream, ID, Question string
		Previous             *readerTurn
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		var sizeError *http.MaxBytesError
		if errors.As(err, &sizeError) {
			http.Error(w, "Request too long", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "Invalid question", http.StatusBadRequest)
		}
		return
	}
	if input.ID == "" {
		http.Error(w, "Reflection ID required", http.StatusBadRequest)
		return
	}
	input.Question = strings.TrimSpace(input.Question)
	if len(input.Question) > server.MaxMessageSize {
		http.Error(w, "Question too long", http.StatusRequestEntityTooLarge)
		return
	}
	if input.Stream == "" {
		input.Stream = server.DefaultStream
	}
	messages := a.server.Retrieve(input.ID, input.Stream, 0, 0, 1)
	if len(messages) == 0 {
		http.NotFound(w, r)
		return
	}
	req := readerRequest(messages[0], input.Question, input.Previous)
	reply, err := a.ask(r.Context(), req)
	if err != nil {
		log.Printf("Mu reader question: %v", err)
		status := http.StatusBadGateway
		var upstream muStatusError
		if errors.As(err, &upstream) {
			switch upstream {
			case http.StatusUnauthorized, http.StatusForbidden, http.StatusPaymentRequired:
				status = http.StatusServiceUnavailable
			case http.StatusTooManyRequests, http.StatusRequestEntityTooLarge:
				status = int(upstream)
			}
		}
		http.Error(w, "Malten could not answer", status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"text": reply.Text})
}

func readerRequest(message *server.Message, question string, previous *readerTurn) muRequest {
	selected := contextMessage{Author: "Selected reflection", Text: message.Text}
	if isDailyPost(message) {
		selected.Author = dailyByline(message)
	}
	req := muRequest{}
	req.Context.Messages = []contextMessage{selected}
	if message.Source != "" {
		req.Context.Messages = append(req.Context.Messages, contextMessage{Author: "Source of the selected reflection", Text: excerpt(message.Source, 640)})
	}
	if previous != nil && strings.TrimSpace(previous.Question) != "" && strings.TrimSpace(previous.Answer) != "" {
		req.Context.Messages = append(req.Context.Messages,
			contextMessage{Author: "Previous reader question", Text: excerpt(previous.Question, 1200)},
			contextMessage{Author: "Previous Malten answer", Text: excerpt(previous.Answer, server.MaxMessageSize)})
		if question == "" {
			question = "Explain your previous answer in more detail."
		}
	}
	if question == "" {
		question = "Explain this reflection in plain language."
	}
	req.Prompt = "Answer the reader about the selected reflection. " + reflectionGuidance + " Use relevant sources available to you when verification is needed and cite what you used.\n\n" + question
	req.addReminderSources(message)
	return req
}
