// Package session keeps paid rooms separate from Malten's public stream APIs.
package session

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/asim/malten/server"
	"github.com/google/uuid"
)

// Receipt is the subset of a Mu Pay booking needed for admission and checkout.
type Receipt struct {
	ID           string `json:"id"`
	Reference    string `json:"reference"`
	Participants int    `json:"participants"`
	Amount       int    `json:"amount"`
	Currency     string `json:"currency"`
	Minutes      int    `json:"minutes"`
	Status       string `json:"status"`
	CheckoutURL  string `json:"checkout_url"`
	EndsAt       int64  `json:"ends_at,omitempty"`
}

type Message struct {
	ID      string `json:"id"`
	Parent  string `json:"parent,omitempty"`
	Author  string `json:"author"`
	Text    string `json:"text"`
	Created int64  `json:"created"`
	HTML    string `json:"html,omitempty"`
}

type Provider interface {
	Available() bool
	Checkout(context.Context, string, int) (Receipt, error)
	Payment(context.Context, string) (Receipt, error)
	StartSession(context.Context, string) (Receipt, error)
	AskSession(context.Context, string, Message, []Message) (string, error)
}

type seat struct {
	Invite string `json:"invite,omitempty"`
	Member string `json:"member,omitempty"`
}

type booking struct {
	Revision       uint64
	StartRequested bool
	Reference      string
	Host           string // Digest of the host's browser credential.
	Participants   int
	Created        int64
	Receipt        Receipt
	Seats          []seat    // The host already occupies the first place.
	Messages       []Message `json:",omitempty"`
}

type room struct {
	op sync.Mutex // Serialize Mu calls for this booking, including the first turn.
	booking
}

type Store struct {
	mu    sync.Mutex
	rooms map[string]*room
	dir   string
	muPay Provider
	now   func() time.Time
}

// Bind Mu's opaque reference to the host. A late payment can be recovered
// after local pending state expires, using the original browser and receipt.
func paymentReference(ref, host string) string {
	mac := hmac.New(sha256.New, []byte(host))
	mac.Write([]byte(ref))
	return ref + "_" + hex.EncodeToString(mac.Sum(nil))
}

func validRef(ref string) bool {
	id, err := uuid.Parse(ref)
	return err == nil && ref == id.String()
}

func Open(dir string, provider Provider) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	s := &Store{rooms: make(map[string]*room), dir: dir, muPay: provider, now: time.Now}
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		if strings.HasPrefix(e.Name(), ".booking-") {
			if err := os.Remove(path); err != nil {
				return nil, err
			}
			continue
		}
		ref, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok || !validRef(ref) {
			continue
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var r room
		if err := json.Unmarshal(b, &r.booking); err != nil {
			return nil, err
		}
		if r.Reference != ref || r.Host == "" || r.Participants < 1 || r.Participants > 100 {
			return nil, errors.New("invalid saved booking " + ref)
		}
		// Old room files had no message IDs. Use the saved position so links
		// stay stable across restarts, even before the next room write.
		for i := range r.Messages {
			if r.Messages[i].ID == "" {
				r.Messages[i].ID = uuid.NewSHA1(uuid.NameSpaceURL, []byte(ref+"/"+strconv.Itoa(i))).String()
			}
		}
		s.rooms[ref] = &r
	}
	if err := s.prune(); err != nil {
		return nil, err
	}
	return s, nil
}

// Call with s.mu held. Publish the in-memory value only after a durable write.
func (s *Store) save(r *room, next booking) error {
	next.Revision++
	tmp, err := os.CreateTemp(s.dir, ".booking-")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	err = json.NewEncoder(tmp).Encode(next)
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(tmp.Name(), filepath.Join(s.dir, next.Reference+".json")); err != nil {
		return err
	}
	r.booking = next
	s.rooms[next.Reference] = r
	return nil
}

func (b booking) status(now int64) string {
	if b.Receipt.Status == "active" && now >= b.Receipt.EndsAt {
		return "expired"
	}
	if b.Receipt.Status == "" {
		return "unconfirmed"
	}
	return b.Receipt.Status
}

func (b booking) member(who string) (int, bool) {
	if who == b.Host {
		return 0, true
	}
	for i, seat := range b.Seats {
		if who == seat.Member {
			return i + 1, true
		}
	}
	return 0, false
}

func (s *Store) apply(r *room, receipt Receipt) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rooms[r.Reference] != r {
		return errMissing
	}
	return s.saveReceipt(r, receipt)
}

// Call with s.mu held.
func (s *Store) saveReceipt(r *room, receipt Receipt) error {
	if receipt.Reference != paymentReference(r.Reference, r.Host) || receipt.Participants != r.Participants ||
		receipt.ID == "" || (r.Receipt.ID != "" && receipt.ID != r.Receipt.ID) ||
		receipt.Amount <= 0 || receipt.Minutes <= 0 || receipt.Minutes > 1440 || (receipt.Currency != "gbp" && receipt.Currency != "usd") {
		return errors.New("Mu returned a different booking or invalid quote")
	}
	if previous := r.Receipt; previous.ID != "" &&
		(previous.Amount != receipt.Amount || previous.Currency != receipt.Currency || previous.Minutes != receipt.Minutes ||
			(previous.EndsAt != 0 && previous.EndsAt != receipt.EndsAt)) {
		return errors.New("Mu changed the booked price, duration or end time")
	}
	if r.Receipt == receipt && !r.StartRequested {
		return nil
	}
	switch receipt.Status {
	case "pending", "paid", "expired":
	case "active":
		if receipt.EndsAt <= 0 {
			return errors.New("Mu returned no end time")
		}
	default:
		return errors.New("unrecognized Mu booking status")
	}
	next := r.booking
	next.Receipt = receipt
	next.StartRequested = false
	if (receipt.Status == "paid" || receipt.Status == "active") && len(next.Seats) == 0 && next.Participants > 1 {
		next.Seats = make([]seat, next.Participants-1)
		for i := range next.Seats {
			next.Seats[i].Invite = rand.Text()
		}
	}
	if next.status(s.now().Unix()) == "expired" {
		next.Messages = nil
		next.Seats = slices.Clone(next.Seats)
		for i := range next.Seats {
			next.Seats[i].Invite = ""
		}
	}
	return s.save(r, next)
}

func (s *Store) prune() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for ref, r := range s.rooms {
		ended := r.Receipt.EndsAt > 0 && now.Unix() >= r.Receipt.EndsAt
		abandoned := (r.Receipt.ID == "" || r.Receipt.Status == "expired") && now.Sub(time.Unix(r.Created, 0)) >= server.StreamTTL
		if ended || abandoned {
			if err := os.Remove(filepath.Join(s.dir, ref+".json")); err != nil {
				return err
			}
			delete(s.rooms, ref)
		}
	}
	return nil
}

// Check abandoned checkouts with Mu before discarding them. A host can return to
// a settled, unstarted booking days later. Transcript expiry needs no network.
func (s *Store) reconcile(ctx context.Context) {
	s.mu.Lock()
	var pending []*room
	for _, r := range s.rooms {
		if r.StartRequested || (r.Receipt.Status == "pending" && s.now().Sub(time.Unix(r.Created, 0)) >= server.StreamTTL) {
			pending = append(pending, r)
		}
	}
	s.mu.Unlock()
	for _, r := range pending {
		if ctx.Err() != nil {
			return
		}
		if err := s.syncPayment(ctx, r, false); err != nil {
			log.Printf("refresh pending booking: %v", err)
		}
	}
}

func (s *Store) Run(ctx context.Context) {
	go func() {
		refresh := time.NewTicker(time.Hour)
		defer refresh.Stop()
		for {
			s.reconcile(ctx)
			select {
			case <-ctx.Done():
				return
			case <-refresh.C:
			}
		}
	}()
	expire := time.NewTicker(time.Second)
	defer expire.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-expire.C:
			if err := s.prune(); err != nil {
				log.Printf("expire paid sessions: %v", err)
			}
		}
	}
}

func (s *Store) syncPayment(ctx context.Context, r *room, start bool) error {
	r.op.Lock()
	defer r.op.Unlock()
	s.mu.Lock()
	b := r.booking
	if s.rooms[b.Reference] != r {
		s.mu.Unlock()
		return errMissing
	}
	if start {
		status := b.status(s.now().Unix())
		if status != "paid" && status != "active" {
			s.mu.Unlock()
			return errInactive
		}
		if status == "paid" && !b.StartRequested {
			// An interrupted Start must be read back from Mu on restart even if
			// the host closes the page before the response arrives.
			b.StartRequested = true
			if err := s.save(r, b); err != nil {
				s.mu.Unlock()
				return err
			}
		}
	}
	s.mu.Unlock()
	var receipt Receipt
	var err error
	switch {
	case start:
		receipt, err = s.muPay.StartSession(ctx, b.Receipt.ID)
	case b.Receipt.ID == "":
		receipt, err = s.muPay.Checkout(ctx, paymentReference(b.Reference, b.Host), b.Participants)
	default:
		receipt, err = s.muPay.Payment(ctx, b.Receipt.ID)
	}
	if err != nil {
		return err
	}
	return s.apply(r, receipt)
}
