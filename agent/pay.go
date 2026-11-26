package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"

	"github.com/asim/malten/session"
)

var muID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func (a *Agent) Available() bool { return a.token != "" }

func (a *Agent) paid(ctx context.Context, method, path string, in any) (session.Receipt, error) {
	var receipt session.Receipt
	if !a.Available() {
		return receipt, errors.New("Mu token is unset")
	}
	var body io.Reader
	if in != nil {
		payload, err := json.Marshal(in)
		if err != nil {
			return receipt, err
		}
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.endpoint+"/pay"+path, body)
	if err != nil {
		return receipt, err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if err := a.do(req, &receipt); err != nil {
		return receipt, err
	}
	base, err := url.Parse(a.endpoint)
	if err != nil {
		return receipt, err
	}
	base.Path = "/pay/" + receipt.ID
	if !muID.MatchString(receipt.ID) || receipt.CheckoutURL != base.String() {
		return session.Receipt{}, errors.New("unexpected checkout URL from Mu")
	}
	return receipt, nil
}

func (a *Agent) Checkout(ctx context.Context, ref string, places int) (session.Receipt, error) {
	return a.paid(ctx, http.MethodPost, "", struct {
		Reference    string `json:"reference"`
		Participants int    `json:"participants"`
	}{ref, places})
}

func (a *Agent) Payment(ctx context.Context, id string) (session.Receipt, error) {
	if !muID.MatchString(id) {
		return session.Receipt{}, errors.New("invalid Mu session ID")
	}
	return a.paid(ctx, http.MethodGet, "/"+id, nil)
}

func (a *Agent) StartSession(ctx context.Context, id string) (session.Receipt, error) {
	if !muID.MatchString(id) {
		return session.Receipt{}, errors.New("invalid Mu session ID")
	}
	return a.paid(ctx, http.MethodPost, "/"+id+"/start", nil)
}

func (a *Agent) AskSession(ctx context.Context, id string, question session.Message, history []session.Message) (string, error) {
	if !a.Available() || !muID.MatchString(id) {
		return "", errors.New("missing paid agent session")
	}
	req := muRequest{Prompt: question.Text, Session: id}
	req.Context.Messages = []contextMessage{{Author: "Malten", Text: "This is a private Malten reflection session. The host and invited guests can read and respond."}}
	if question.Parent != "" {
		for _, m := range history {
			if m.ID == question.Parent {
				req.addQuote(m.Text, m.Author)
				break
			}
		}
	}
	base, _ := json.Marshal(req)
	encodedContext, _ := json.Marshal(req.Context)
	budget := min(15_000-len(encodedContext), muRequestBytes-len(base))
	var recent []contextMessage
	for i := len(history) - 1; i >= 0 && len(recent) < 6; i-- {
		if question.Parent != "" && history[i].ID == question.Parent {
			continue
		}
		m := contextMessage{Author: history[i].Author, Text: history[i].Text}
		encoded, _ := json.Marshal(m)
		if len(encoded)+1 > budget {
			break
		}
		budget -= len(encoded) + 1
		recent = append(recent, m)
	}
	slices.Reverse(recent)
	req.Context.Messages = append(req.Context.Messages, recent...)
	reply, err := a.ask(ctx, req)
	return reply.Text, err
}
