// Package server implements the Malten stream server.
//
// ARCHITECTURE (read ARCHITECTURE.md and claude.md "The Spacetime Model"):
//
// Streams are spacetime - events propagate through them to observers.
// Channels filter conversations (like radio frequencies in same space).
//
// PRIVACY RULE:
//   channel = ""        -> public broadcast (can persist to events.jsonl)
//   channel = "@session" -> private to user (NEVER persist, belongs in localStorage)
//   channel = "#group"   -> shared group (future)
//
// User messages are stored in-memory for real-time delivery but NOT persisted.
// The user's localStorage is their source of truth for conversation history.
//
package server

import (
	"crypto/rand"
	"errors"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/google/uuid"
)

const (
	MaxMessageSize = 1024
	MaxMessages    = 1024
	StreamTTL      = 24 * time.Hour
)

type Metadata struct {
	Created     int64
	Title       string
	Description string
	Type        string
	Image       string
	Url         string
	Site        string
}

type Stream struct {
	// A unique id
	Id string
	// A secret for access
	Secret string
	// Messages in the stream
	Messages []*Message
	// not listed
	Private bool
	// In nanoseconds
	Updated int64
	// In seconds
	TTL int64
	// stream observers
	Observers int64
}

type Message struct {
	Id        string
	Text      string
	Type      string
	Created   int64 `json:",string"`
	Stream    string
	Channel   string // "" = public, "@session" = addressed
	CommandID string `json:",omitempty"` // For async command results
	Metadata  *Metadata
}

type Observer struct {
	Id      string
	Stream  string
	Session string // session token for channel filtering
	Events  chan *Message
	Kill    chan bool
}

type Server struct {
	Created int64
	Events  chan *Message

	mtx       sync.RWMutex
	streams   map[string]*Stream
	metadata  map[string]*Metadata
	observers map[string]*Observer
}

var (
	alphanum = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
)

var Default = New()

// random generate i length alphanum string
func Random(i int) string {
	bytes := make([]byte, i)
	for {
		rand.Read(bytes)
		for i, b := range bytes {
			bytes[i] = alphanum[b%byte(len(alphanum))]
		}
		return string(bytes)
	}
	return uuid.New().String()
}

func New() *Server {
	return &Server{
		Created:   time.Now().UnixNano(),
		Events:    make(chan *Message, 100),
		streams:   make(map[string]*Stream),
		metadata:  make(map[string]*Metadata),
		observers: make(map[string]*Observer),
	}
}

func NewCommand(prompt, stream string) *Message {
	return &Message{
		Id:      uuid.New().String(),
		Text:    prompt,
		Type:    "command",
		Created: time.Now().UnixNano(),
		Stream:  stream,
	}
}

func NewStream(id, secret string, private bool, ttl int) *Stream {
	return &Stream{
		Id:      id,
		Secret:  secret,
		Private: private,
		Updated: time.Now().UnixNano(),
		TTL:     (time.Duration(ttl) * time.Second).Nanoseconds(),
	}
}

func NewMessage(text, stream string) *Message {
	return NewChannelMessage(text, stream, "")
}

func NewChannelMessage(text, stream, channel string) *Message {
	return &Message{
		Id:      uuid.New().String(),
		Text:    text,
		Type:    "message",
		Created: time.Now().UnixNano(),
		Stream:  stream,
		Channel: channel,
	}
}

// NewCommandResult creates a message for an async command result
func NewCommandResult(cmdID, text, stream, channel string) *Message {
	return &Message{
		Id:        uuid.New().String(),
		Text:      text,
		Type:      "command_result",
		Created:   time.Now().UnixNano(),
		Stream:    stream,
		Channel:   channel,
		CommandID: cmdID,
	}
}

func NewEvent(text, stream string) *Message {
	return &Message{
		Id:      uuid.New().String(),
		Text:    text,
		Type:    "event",
		Created: time.Now().UnixNano(),
		Stream:  stream,
	}
}

func NewObserver(stream, session string) *Observer {
	return &Observer{
		Id:      uuid.New().String(),
		Events:  make(chan *Message, 1),
		Kill:    make(chan bool),
		Stream:  stream,
		Session: session,
	}
}

func GetMetadata(uri string) *Metadata {
	u, err := url.Parse(uri)
	if err != nil {
		return nil
	}

	d, err := goquery.NewDocument(u.String())
	if err != nil {
		return nil
	}

	g := &Metadata{
		Created: time.Now().UnixNano(),
	}

	for _, node := range d.Find("meta").Nodes {
		if len(node.Attr) < 2 {
			continue
		}

		p := strings.Split(node.Attr[0].Val, ":")
		if len(p) < 2 || (p[0] != "twitter" && p[0] != "og") {
			continue
		}

		switch p[1] {
		case "site_name":
			g.Site = node.Attr[1].Val
		case "site":
			if len(g.Site) == 0 {
				g.Site = node.Attr[1].Val
			}
		case "title":
			g.Title = node.Attr[1].Val
		case "description":
			g.Description = node.Attr[1].Val
		case "card", "type":
			g.Type = node.Attr[1].Val
		case "url":
			g.Url = node.Attr[1].Val
		case "image":
			if len(p) > 2 && p[2] == "src" {
				g.Image = node.Attr[1].Val
			} else if len(g.Image) == 0 {
				g.Image = node.Attr[1].Val
			}
		}
	}

	if len(g.Type) == 0 || len(g.Image) == 0 || len(g.Title) == 0 || len(g.Url) == 0 {
		return nil
	}

	return g
}

func (s *Server) Broadcast(message *Message) {
	var observers []*Observer

	s.mtx.RLock()
	for _, o := range s.observers {
		observers = append(observers, o)
	}
	s.mtx.RUnlock()

	for _, o := range observers {
		// only broadcast what they care about
		if message.Stream != o.Stream {
			continue
		}
		// Filter by channel: public (empty) or addressed to this observer's session
		if message.Channel != "" && message.Channel != "@"+o.Session {
			continue
		}
		// send message
		select {
		case o.Events <- message:
		default:
		}
	}
}

func (s *Server) New(stream, secret string, private bool, ttl int) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if _, ok := s.streams[stream]; ok {
		return errors.New("already exists")
	}

	str := NewStream(stream, secret, private, ttl)
	s.streams[str.Id] = str

	return nil
}

func (s *Server) Metadata(t *Message) {
	parts := strings.Split(t.Text, " ")
	for _, part := range parts {
		g := GetMetadata(part)
		if g == nil {
			continue
		}
		s.mtx.Lock()
		s.metadata[t.Id] = g
		s.mtx.Unlock()
		return
	}
}

func (s *Server) List() map[string]*Stream {
	s.mtx.RLock()
	streams := s.streams
	s.mtx.RUnlock()
	return streams
}

func (s *Server) Observe(o *Observer) {
	s.mtx.Lock()
	s.observers[o.Id] = o

	st, ok := s.streams[o.Stream]
	if !ok {
		st = NewStream(o.Stream, "", false, int(StreamTTL.Seconds()))
	}

	// update observer count
	st.Observers++
	s.streams[o.Stream] = st

	s.mtx.Unlock()

	// send connect event
	s.Events <- NewEvent("connect", o.Stream)

	go func() {
		<-o.Kill
		s.mtx.Lock()
		delete(s.observers, o.Id)

		// update observer count
		st, ok := s.streams[o.Stream]
		if ok {
			st.Observers--
			s.streams[o.Stream] = st
		}

		s.mtx.Unlock()

		// send disconnect event
		s.Events <- NewEvent("close", o.Stream)
	}()
}

func (s *Server) Store(message *Message) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	// check the listing thing
	stream, ok := s.streams[message.Stream]
	if !ok {
		// default to a private stream
		stream = NewStream(message.Stream, "", true, int(StreamTTL.Seconds()))
		s.streams[stream.Id] = stream
	}

	stream.Messages = append(stream.Messages, message)
	if len(stream.Messages) > MaxMessages {
		stream.Messages = stream.Messages[1:]
	}

	stream.Updated = time.Now().UnixNano()
}

func (s *Server) Retrieve(message string, streem string, direction, last, limit int64) []*Message {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	var stream *Stream

	stream, ok := s.streams[streem]
	if !ok {
		return []*Message{}
	}

	if len(message) == 0 {
		var messages []*Message

		if limit <= 0 {
			return messages
		}

		li := int(limit)

		// go back in time
		if direction < 0 {
			for i := len(stream.Messages) - 1; i >= 0; i-- {
				if len(messages) >= li {
					return messages
				}

				message := stream.Messages[i]

				if message.Created < last {
					if g, ok := s.metadata[message.Id]; ok {
						tc := *message
						tc.Metadata = g
						messages = append(messages, &tc)
					} else {
						messages = append(messages, message)
					}
				}
			}
			return messages
		}

		start := 0
		if len(stream.Messages) > li {
			start = len(stream.Messages) - li
		}

		for i := start; i < len(stream.Messages); i++ {
			if len(messages) >= li {
				return messages
			}

			message := stream.Messages[i]

			if message.Created > last {
				if g, ok := s.metadata[message.Id]; ok {
					tc := *message
					tc.Metadata = g
					messages = append(messages, &tc)
				} else {
					messages = append(messages, message)
				}
			}
		}
		return messages
	}

	// retrieve one
	for _, t := range stream.Messages {
		var messages []*Message
		if message == t.Id {
			if g, ok := s.metadata[t.Id]; ok {
				tc := *t
				tc.Metadata = g
				messages = append(messages, &tc)
			} else {
				messages = append(messages, t)
			}
			return messages
		}
	}

	return []*Message{}
}

// RetrieveForSession gets messages visible to a session (public + their @channel)
func (s *Server) RetrieveForSession(streem, session string, last, limit int64) []*Message {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	stream, ok := s.streams[streem]
	if !ok {
		return []*Message{}
	}

	var messages []*Message
	li := int(limit)
	if li <= 0 {
		li = 25
	}

	channel := "@" + session

	// Go through messages, filter by visibility
	for i := len(stream.Messages) - 1; i >= 0 && len(messages) < li; i-- {
		msg := stream.Messages[i]
		if msg.Created <= last {
			continue
		}
		// Visible if: public (no channel) OR addressed to this session
		if msg.Channel == "" || msg.Channel == channel {
			if g, ok := s.metadata[msg.Id]; ok {
				tc := *msg
				tc.Metadata = g
				messages = append(messages, &tc)
			} else {
				messages = append(messages, msg)
			}
		}
	}

	// Reverse to chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages
}

func (s *Server) Run() {
	t1 := time.NewTicker(time.Second)

	for {
		select {
		case message := <-s.Events:
			if message.Type == "message" {
				s.Store(message)
				go s.Metadata(message)
				// Note: User messages are NOT persisted to events.jsonl
				// Per spacetime model: user conversations belong in localStorage (their worldline)
				// events.jsonl is for facts about the world, not private experiences
			}
			go s.Broadcast(message)
		case <-t1.C:
			now := time.Now().UnixNano()

			s.mtx.Lock()

			// delete the metadata
			for metadata, g := range s.metadata {
				if d := now - g.Created; d > StreamTTL.Nanoseconds() {
					delete(s.metadata, metadata)
				}
			}

			// TODO: make a copy and replace the map as its not GC'ed
			for name, stream := range s.streams {
				// time since last update in nano seconds
				d := now - stream.Updated
				// delete older than the TTL
				if d > stream.TTL {
					delete(s.streams, name)
					stream.Messages = nil
					continue
				}

				// expire old messages
				var messages []*Message
				for _, message := range stream.Messages {
					d := now - message.Created
					if d > stream.TTL {
						continue
					}
					messages = append(messages, message)
				}
				// update stream messages
				stream.Messages = messages
			}

			s.mtx.Unlock()
		}
	}
}

func Run() {
	Default.Run()
}
