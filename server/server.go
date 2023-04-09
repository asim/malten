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
	StreamTTL      = time.Duration(1024) * time.Second
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
	Id       string
	Messages []*Message
	Private  bool
	// In nanoseconds
	Updated int64
	// In seconds
	TTL       int64
	Observers int64
}

type Message struct {
	Id       string
	Text     string
	Type     string
	Created  int64 `json:",string"`
	Stream   string
	Metadata *Metadata
}

type Observer struct {
	Id     string
	Events chan *Message
	Kill   chan bool
	Stream string
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

func newStream(id string, private bool, ttl int) *Stream {
	return &Stream{
		Id:      id,
		Private: private,
		Updated: time.Now().UnixNano(),
		TTL:     (time.Duration(ttl) * time.Second).Nanoseconds(),
	}
}

func NewMessage(text, stream string) *Message {
	return &Message{
		Id:      uuid.New().String(),
		Text:    text,
		Type:    "message",
		Created: time.Now().UnixNano(),
		Stream:  stream,
	}
}

func newEvent(text, stream string) *Message {
	return &Message{
		Id:      uuid.New().String(),
		Text:    text,
		Type:    "event",
		Created: time.Now().UnixNano(),
		Stream:  stream,
	}
}

func getMetadata(uri string) *Metadata {
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
		// send message
		select {
		case o.Events <- message:
		default:
		}
	}
}

func (s *Server) New(stream string, private bool, ttl int) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if _, ok := s.streams[stream]; ok {
		return errors.New("already exists")
	}

	str := newStream(stream, private, ttl)
	s.streams[str.Id] = str

	return nil
}

func (c *Server) Metadata(t *Message) {
	parts := strings.Split(t.Text, " ")
	for _, part := range parts {
		g := getMetadata(part)
		if g == nil {
			continue
		}
		c.mtx.Lock()
		c.metadata[t.Id] = g
		c.mtx.Unlock()
		return
	}
}

func (c *Server) List() map[string]*Stream {
	c.mtx.RLock()
	streams := c.streams
	c.mtx.RUnlock()
	return streams
}

func (c *Server) Observe(o *Observer) {
	c.mtx.Lock()
	c.observers[o.Id] = o

	s, ok := c.streams[o.Stream]
	if !ok {
		s = newStream(o.Stream, false, int(StreamTTL.Seconds()))
	}

	// update observer count
	s.Observers++
	c.streams[o.Stream] = s

	c.mtx.Unlock()

	// send connect event
	c.Events <- newEvent("connect", o.Stream)

	go func() {
		<-o.Kill
		c.mtx.Lock()
		delete(c.observers, o.Id)

		// update observer count
		s, ok := c.streams[o.Stream]
		if ok {
			s.Observers--
			c.streams[o.Stream] = s
		}

		c.mtx.Unlock()

		// send disconnect event
		c.Events <- newEvent("close", o.Stream)
	}()
}

func (s *Server) Save(message *Message) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	// check the listing thing
	stream, ok := s.streams[message.Stream]
	if !ok {
		stream = newStream(message.Stream, false, int(StreamTTL.Seconds()))
		s.streams[stream.Id] = stream
	}

	stream.Messages = append(stream.Messages, message)
	if len(stream.Messages) > MaxMessages {
		stream.Messages = stream.Messages[1:]
	}

	stream.Updated = time.Now().UnixNano()
}

func (c *Server) Retrieve(message string, streem string, direction, last, limit int64) []*Message {
	c.mtx.RLock()
	defer c.mtx.RUnlock()

	var stream *Stream

	stream, ok := c.streams[streem]
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
					if g, ok := c.metadata[message.Id]; ok {
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
				if g, ok := c.metadata[message.Id]; ok {
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
			if g, ok := c.metadata[t.Id]; ok {
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

func (s *Server) Run() {
	t1 := time.NewTicker(time.Second)

	for {
		select {
		case message := <-s.Events:
			if message.Type == "message" {
				s.Save(message)
				go s.Metadata(message)
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
