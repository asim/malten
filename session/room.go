package session

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/subtle"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/asim/malten/server"
	"github.com/yuin/goldmark"
)

func (s *Store) reserve(who string, in input) (*room, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r := s.rooms[in.Room]; r != nil {
		if r.Host != who {
			return nil, errMissing
		}
		if r.Participants != in.Participants {
			return nil, clientError{409, "The number of places for this booking is fixed."}
		}
		return r, nil
	}

	// Reopening a quote in another tab should not create a second Stripe checkout.
	now := s.now()
	drafts := 0
	for _, r := range s.rooms {
		if r.Receipt.ID == "" {
			drafts++
		}
		if r.Host == who && r.Participants == in.Participants &&
			(r.Receipt.ID == "" || r.Receipt.Status == "pending") && now.Sub(time.Unix(r.Created, 0)) < time.Hour {
			return r, nil
		}
	}
	// Mu caps pending checkouts at 100 per owner. Bound local unconfirmed
	// attempts too, including when a checkout request times out.
	if drafts >= 100 {
		return nil, clientError{429, "There are too many open checkouts. Try again later."}
	}
	r := new(room)
	err := s.save(r, booking{Reference: in.Room, Host: who, Participants: in.Participants, Created: now.Unix()})
	return r, err
}

func (s *Store) book(ctx context.Context, who string, in input) (any, error) {
	if !s.muPay.Available() {
		return nil, clientError{503, "Booking is unavailable."}
	}
	if !validRef(in.Room) || in.Participants < 1 || in.Participants > 100 {
		return nil, clientError{400, "Choose between 1 and 100 places, including yours."}
	}
	r, err := s.reserve(who, in)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	ref, confirmed := r.Reference, r.Receipt.ID != ""
	s.mu.Unlock()
	if !confirmed {
		if err := s.syncPayment(ctx, r, false); err != nil {
			return nil, err
		}
	}
	return s.state(who, ref)
}

func (s *Store) claim(r *room, who, invite string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := r.status(s.now().Unix())
	if s.rooms[r.Reference] != r || (status != "paid" && status != "active") {
		return errInvite
	}
	if _, ok := r.member(who); ok {
		return nil
	}
	for i, place := range r.Seats {
		if place.Invite == "" || subtle.ConstantTimeCompare([]byte(place.Invite), []byte(invite)) != 1 {
			continue
		}
		b := r.booking
		b.Seats = slices.Clone(b.Seats)
		b.Seats[i] = seat{Member: who}
		return s.save(r, b)
	}
	return errInvite
}

func (s *Store) append(r *room, author, text string) error {
	if s.rooms[r.Reference] != r || r.status(s.now().Unix()) != "active" {
		return errInactive
	}
	b := r.booking
	m := Message{Author: author, Text: text, Created: s.now().Unix()}
	if author == "Malten" {
		var html bytes.Buffer
		if err := goldmark.Convert([]byte(text), &html); err != nil {
			return err
		}
		m.HTML = html.String()
	}
	b.Messages = append(slices.Clone(b.Messages[max(0, len(b.Messages)-server.MaxMessages+1):]), m)
	return s.save(r, b)
}

func (s *Store) post(ctx context.Context, r *room, who string, in input) error {
	body := strings.TrimSpace(in.Text)
	prompt := body
	first := body
	if i := strings.IndexFunc(body, unicode.IsSpace); i >= 0 {
		first = body[:i]
	}
	if strings.EqualFold(first, "@malten") {
		in.Ask, prompt = true, strings.TrimSpace(body[len(first):])
	}
	if prompt == "" || len(body) > server.MaxMessageSize {
		return clientError{400, "Write a message of up to 4,096 UTF-8 bytes."}
	}
	if in.Ask {
		r.op.Lock()
		defer r.op.Unlock()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	place, ok := r.member(who)
	if !ok {
		s.mu.Unlock()
		return errMissing
	}
	author := "Host"
	if place > 0 {
		author = fmt.Sprintf("Guest %d", place)
	}
	id, history := r.Receipt.ID, r.Messages
	err := s.append(r, author, body)
	s.mu.Unlock()
	if err != nil || !in.Ask {
		return err
	}
	reply, err := s.muPay.AskSession(ctx, id, prompt, history)
	if err != nil {
		log.Printf("paid agent reply: %v", err)
		return errReply
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.append(r, "Malten", server.NewMessage(reply, "").Text)
}

// A checkout redirect carries a public lookup ID. Only Mu's authenticated
// receipt and the original browser can recover this reservation.
func (s *Store) checkoutReturn(ctx context.Context, who, id string) (any, error) {
	if id == "" || len(id) > 128 {
		return nil, errMissing
	}
	s.mu.Lock()
	for _, r := range s.rooms {
		if r.Receipt.ID != id {
			continue
		}
		ref, owner := r.Reference, r.Host
		s.mu.Unlock()
		if owner != who {
			return nil, errMissing
		}
		if err := s.syncPayment(ctx, r, false); err != nil {
			return nil, err
		}
		return s.state(who, ref)
	}
	s.mu.Unlock()
	receipt, err := s.muPay.Payment(ctx, id)
	if err != nil {
		return nil, errMissing
	}
	ref, _, _ := strings.Cut(receipt.Reference, "_")
	if receipt.ID != id || !validRef(ref) || !hmac.Equal([]byte(receipt.Reference), []byte(paymentReference(ref, who))) {
		return nil, errMissing
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if r := s.rooms[ref]; r != nil {
		if r.Host != who || r.Receipt.ID != id {
			return nil, errMissing
		}
		return s.view(r, who, true), nil
	}
	// Only unstarted paid reservations can be recovered from Mu alone. Active
	// rooms also require their locally persisted seat assignments and end time.
	if receipt.Status != "paid" || receipt.Participants < 1 || receipt.Participants > 100 {
		return nil, errMissing
	}
	r := &room{booking: booking{Reference: ref, Host: who, Participants: receipt.Participants, Created: s.now().Unix()}}
	if err := s.saveReceipt(r, receipt); err != nil {
		return nil, err
	}
	return s.view(r, who, true), nil
}
