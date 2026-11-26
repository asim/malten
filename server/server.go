package server

import (
	"crypto/rand"
	"errors"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
	"github.com/google/uuid"
)

const (
	MaxMessageSize = 4 * 1024
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
	// In nanoseconds
	TTL int64
	// Private continuation state, included only in the recovery snapshot.
	Thread        string `json:",omitempty"`
	ThreadExpires int64  `json:",omitempty"`
	// stream observers
	Observers int64
}

// Quote is a read-time excerpt of the live parent. Expires is Unix milliseconds.
type Quote struct {
	Text    string
	Source  string `json:",omitempty"`
	Expires int64
}

// Reference identifies a reading that inspired a message.
type Reference struct {
	Title string
	Text  string `json:",omitempty"`
	URL   string `json:",omitempty"`
}

type Message struct {
	Observers int `json:",omitempty"`
	Source    string
	Parent    string `json:",omitempty"`
	// Quote and Expires are populated on copies sent to readers.
	Quote      *Quote `json:",omitempty"`
	Expires    int64  `json:",omitempty"`
	Id         string
	Text       string
	Type       string
	Created    int64 `json:",string"`
	Stream     string
	Metadata   *Metadata
	References []Reference `json:",omitempty"`
}

type Observer struct {
	Id     string
	Stream string
	Events chan *Message
	Kill   chan bool
}

type Server struct {
	Created  int64
	Events   chan *Message
	Commands chan *Message

	commandMu      sync.Mutex
	statePath      string
	nextDiskExpiry int64
	dirty          bool
	mtx            sync.RWMutex
	streams        map[string]*Stream
	metadata       map[string]*Metadata
	observers      map[string]*Observer
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
	if len(text) > MaxMessageSize {
		const suffix = "…"
		end := MaxMessageSize - len(suffix)
		for !utf8.RuneStart(text[end]) {
			end--
		}
		text = text[:end] + suffix
	}
	return &Message{
		Id:      uuid.New().String(),
		Text:    text,
		Type:    "message",
		Created: time.Now().UnixNano(),
		Stream:  stream,
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

func NewObserver(stream string) *Observer {
	return &Observer{
		Id:     uuid.New().String(),
		Events: make(chan *Message, 1),
		Kill:   make(chan bool),
		Stream: stream,
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
		if o.Stream == message.Stream {
			observers = append(observers, o)
		}
	}
	if st := s.streams[message.Stream]; len(observers) > 0 && st != nil && message.Type != "event" {
		batch := []*Message{message}
		s.forReadersLocked(st, batch, time.Now().UnixNano())
		message = batch[0]
	}
	s.mtx.RUnlock()

	if message.Type == "event" {
		message.Observers = len(observers)
	}

	for _, o := range observers {
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
	s.dirty = true

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
	defer s.mtx.RUnlock()
	result := make(map[string]*Stream, len(s.streams))
	for name, stream := range s.streams {
		result[name] = &Stream{Id: stream.Id, Private: stream.Private, Updated: stream.Updated, TTL: stream.TTL, Observers: stream.Observers}
	}
	return result
}

func (s *Server) Observe(o *Observer) {
	s.mtx.Lock()
	s.observers[o.Id] = o

	st, ok := s.streams[o.Stream]
	if !ok {
		s.dirty = true
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

func (s *Server) store(message *Message) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	return s.storeLocked(message)
}

var errDuplicate = errors.New("message already exists")
var errParentGone = errors.New("The original reflection is no longer in this stream.")

func (st *Stream) liveMessage(id string, now int64) *Message {
	if st != nil {
		for _, m := range st.Messages {
			if m.Id == id && now-m.Created < st.TTL {
				return m
			}
		}
	}
	return nil
}

func (s *Server) storeLocked(message *Message) error {
	stream := s.streams[message.Stream]
	if stream != nil {
		for _, previous := range stream.Messages {
			if previous.Id == message.Id {
				return errDuplicate
			}
		}
	}
	now := time.Now().UnixNano()
	if message.Parent != "" && stream.liveMessage(message.Parent, now) == nil {
		return errParentGone
	}
	if stream == nil {
		// default to a private stream
		stream = NewStream(message.Stream, "", message.Stream != DefaultStream, int(StreamTTL.Seconds()))
		s.streams[stream.Id] = stream
	}

	// A caller can republish a retrieved message. Store only its own content.
	if message.Quote != nil || message.Expires != 0 {
		copy := *message
		copy.Quote, copy.Expires = nil, 0
		message = &copy
	}
	s.dirty = true
	stream.Messages = append(stream.Messages, message)
	if len(stream.Messages) > MaxMessages {
		stream.Messages = stream.Messages[1:]
	}

	stream.Updated = now
	return nil
}

// forReadersLocked resolves all parents in one pass, including those outside
// the requested page. A quote never becomes part of the stored child.
func (s *Server) forReadersLocked(st *Stream, messages []*Message, now int64) {
	if len(messages) == 0 {
		return
	}
	parents := make(map[string]*Message)
	for _, m := range messages {
		if m.Parent != "" {
			parents[m.Parent] = nil
		}
	}
	if len(parents) != 0 {
		for _, m := range st.Messages {
			if _, wanted := parents[m.Id]; wanted && now-m.Created < st.TTL {
				parents[m.Id] = m
			}
		}
	}
	for i, m := range messages {
		view := *m
		view.Quote = nil
		view.Expires = time.Unix(0, m.Created+st.TTL).UnixMilli()
		if meta := s.metadata[m.Id]; meta != nil {
			view.Metadata = meta
		}
		if p := parents[m.Parent]; p != nil {
			text := p.Text
			if p.Type == "command" {
				text = "@malten " + text
			}
			if chars := []rune(text); len(chars) > 180 {
				text = string(chars[:179]) + "…"
			}
			view.Quote = &Quote{Text: text, Source: p.Source, Expires: time.Unix(0, p.Created+st.TTL).UnixMilli()}
		}
		messages[i] = &view
	}
}

func (s *Server) Retrieve(id, stream string, direction, last, limit int64) []*Message {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	st := s.streams[stream]
	if st == nil {
		return []*Message{}
	}
	now := time.Now().UnixNano()
	var messages []*Message
	switch {
	case id != "":
		if m := st.liveMessage(id, now); m != nil {
			messages = append(messages, m)
		} else {
			return []*Message{}
		}
	case limit <= 0:
		return nil
	case direction < 0:
		for i := len(st.Messages) - 1; i >= 0 && int64(len(messages)) < limit; i-- {
			m := st.Messages[i]
			if m.Created < last && now-m.Created < st.TTL {
				messages = append(messages, m)
			}
		}
	default:
		start := max(0, int64(len(st.Messages))-limit)
		for _, m := range st.Messages[start:] {
			if m.Created > last && now-m.Created < st.TTL {
				messages = append(messages, m)
			}
		}
	}
	s.forReadersLocked(st, messages, now)
	return messages
}

func (s *Server) Publish(message *Message) error {
	if err := s.store(message); err != nil {
		if err == errDuplicate {
			return nil
		}
		return err
	}
	if message.Source == "" {
		go s.Metadata(message)
	}
	s.Broadcast(message)
	return nil
}

func (s *Server) Run() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case message := <-s.Events:
			if message.Type == "message" {
				s.Publish(message)
			} else {
				s.Broadcast(message)
			}
		case <-ticker.C:
			if err := s.Checkpoint(); err != nil {
				log.Printf("Saving streams: %v", err)
			}
		}
	}
}

func Run() { Default.Run() }

// SubmitCommand stores the question before the worker can publish its reply.
func (s *Server) SubmitCommand(command *Message) bool {
	return s.queueCommand(command, command.Text) == nil
}

func (s *Server) queueCommand(message *Message, prompt string) error {
	s.commandMu.Lock()
	defer s.commandMu.Unlock()
	if len(s.Commands) >= cap(s.Commands) {
		return errAgentUnavailable
	}
	if err := s.store(message); err != nil {
		if err == errDuplicate {
			return nil
		}
		return err
	}
	s.Broadcast(message)
	command := *message
	command.Text = prompt
	s.Commands <- &command
	return nil
}

func (s *Server) Thread(stream string) string {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	if st := s.streams[stream]; st != nil && st.ThreadExpires > time.Now().UnixNano() {
		return st.Thread
	}
	return ""
}

// Reply keeps the answer and its continuation in the same checkpoint.
func (s *Server) Reply(command *Message, text, source, thread string) {
	reply := NewMessage(text, command.Stream)
	reply.Source = source
	reply.Parent = command.Id
	s.mtx.Lock()
	if err := s.storeLocked(reply); err != nil {
		s.mtx.Unlock()
		return
	}
	if thread != "" {
		st := s.streams[command.Stream]
		st.Thread = thread
		st.ThreadExpires = reply.Created + min(st.TTL, int64(StreamTTL))
	}
	s.mtx.Unlock()
	s.Broadcast(reply)
}
