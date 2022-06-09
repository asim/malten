package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/golang/groupcache/lru"
	"github.com/google/uuid"
)

const (
	defaultStream  = "_"
	maxMessageSize = 512
	maxMessages    = 1000
	maxStreams     = 1000
	streamTTL      = 8.64e13
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
	Updated  int64
}

type Message struct {
	Id       string
	Text     string
	Created  int64 `json:",string"`
	Stream   string
	Metadata *Metadata
}

type Server struct {
	Created int64
	Updates chan *Message

	mtx      sync.RWMutex
	Streams  *lru.Cache
	streams  map[string]int64
	metadata map[string]*Metadata
}

//go:embed html/*
var html embed.FS

var (
	S = newServer()
)

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
}

func newServer() *Server {
	return &Server{
		Created:  time.Now().UnixNano(),
		Streams:  lru.New(maxStreams),
		Updates:  make(chan *Message, 100),
		streams:  make(map[string]int64),
		metadata: make(map[string]*Metadata),
	}
}

func newStream(id string) *Stream {
	return &Stream{
		Id:      id,
		Updated: time.Now().UnixNano(),
	}
}

func newMessage(text, stream string) *Message {
	return &Message{
		Id:      uuid.New().String(),
		Text:    text,
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

func getHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	message := r.Form.Get("id")
	stream := r.Form.Get("stream")

	last, err := strconv.ParseInt(r.Form.Get("last"), 10, 64)
	if err != nil {
		last = 0
	}

	limit, err := strconv.ParseInt(r.Form.Get("limit"), 10, 64)
	if err != nil {
		limit = 25
	}

	direction, err := strconv.ParseInt(r.Form.Get("direction"), 10, 64)
	if err != nil {
		direction = 1
	}

	// default stream
	if len(stream) == 0 {
		stream = defaultStream
	}

	messages := S.Retrieve(message, stream, direction, last, limit)
	b, _ := json.Marshal(messages)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, string(b))
}

func getStreamsHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	b, _ := json.Marshal(S.List())
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, string(b))
}

func postHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	message := r.Form.Get("text")
	stream := r.Form.Get("stream")

	if len(message) == 0 {
		http.Error(w, "Message cannot be blank", 400)
		return
	}

	// default stream
	if len(stream) == 0 {
		stream = defaultStream
	}

	// default length
	if len(message) > maxMessageSize {
		message = message[:maxMessageSize]
	}

	select {
	case S.Updates <- newMessage(message, stream):
	case <-time.After(time.Second):
		http.Error(w, "Timed out creating message", 504)
	}
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

func (c *Server) List() map[string]int64 {
	c.mtx.RLock()
	streams := c.streams
	c.mtx.RUnlock()
	return streams
}

func (c *Server) Save(message *Message) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	var stream *Stream

	if object, ok := c.Streams.Get(message.Stream); ok {
		stream = object.(*Stream)
	} else {
		stream = newStream(message.Stream)
		c.Streams.Add(message.Stream, stream)
	}

	stream.Messages = append(stream.Messages, message)
	if len(stream.Messages) > maxMessages {
		stream.Messages = stream.Messages[1:]
	}
	stream.Updated = time.Now().UnixNano()
}

func (c *Server) Retrieve(message string, streem string, direction, last, limit int64) []*Message {
	c.mtx.RLock()
	defer c.mtx.RUnlock()

	var stream *Stream

	if object, ok := c.Streams.Get(streem); ok {
		stream = object.(*Stream)
	} else {
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

func (c *Server) Start() {
	t1 := time.NewTicker(time.Hour)
	t2 := time.NewTicker(time.Minute)
	streams := make(map[string]int64)

	for {
		select {
		case message := <-c.Updates:
			c.Save(message)
			streams[message.Stream] = time.Now().UnixNano()
			go c.Metadata(message)
		case <-t1.C:
			now := time.Now().UnixNano()
			for stream, u := range streams {
				if d := now - u; d > streamTTL {
					c.Streams.Remove(stream)
					delete(streams, stream)
				}
			}
			c.mtx.Lock()
			for metadata, g := range c.metadata {
				if d := now - g.Created; d > streamTTL {
					delete(c.metadata, metadata)
				}
			}
			c.mtx.Unlock()
		case <-t2.C:
			c.mtx.Lock()
			c.streams = streams
			c.mtx.Unlock()
		}
	}
}

func main() {
	go S.Start()

	htmlContent, err := fs.Sub(html, "html")
	if err != nil {
		log.Fatal(err)
	}

	// serve the html directory by default
	http.Handle("/", http.FileServer(http.FS(htmlContent)))

	http.HandleFunc("/streams", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			getStreamsHandler(w, r)
		default:
			http.Error(w, "unsupported method "+r.Method, 400)
		}
	})

	http.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			getHandler(w, r)
		case "POST":
			postHandler(w, r)
		default:
			http.Error(w, "unsupported method "+r.Method, 400)
		}
	})

	http.ListenAndServe(":9090", nil)
}
