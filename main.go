package main

import (
	"crypto/rand"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/google/uuid"
)

const (
	defaultStream  = "_"
	maxMessageSize = 1024
	maxMessages    = 1024
	streamTTL      = time.Duration(1024) * time.Second
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
	TTL int64
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
	Events  chan *Message

	mtx      sync.RWMutex
	streams  map[string]*Stream
	metadata map[string]*Metadata
}

//go:embed html/*
var html embed.FS

var (
	alphanum = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

	S = newServer()
)

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
}

// random generate i length alphanum string
func random(i int) string {
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

func newServer() *Server {
	return &Server{
		Created:  time.Now().UnixNano(),
		Events:   make(chan *Message, 100),
		streams:  make(map[string]*Stream),
		metadata: make(map[string]*Metadata),
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

	streams := make(map[string]int64)

	for k, v := range S.List() {
		// only return public streams
		if v.Private {
			continue
		}
		streams[k] = v.Updated
	}

	b, _ := json.Marshal(streams)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, string(b))
}

func newStreamHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	stream := r.Form.Get("stream")
	private, _ := strconv.ParseBool(r.Form.Get("private"))
	ttl, _ := strconv.Atoi(r.Form.Get("ttl"))

	if len(stream) == 0 {
		stream = random(8)
	}

	if ttl <= 0 {
		ttl = int(streamTTL.Seconds())
	}

	if err := S.New(stream, private, ttl); err != nil {
		http.Error(w, "Cannot create stream", 500)
		return
	}

	data := map[string]interface{}{
		"stream":  stream,
		"private": private,
		"ttl":     ttl,
	}
	b, _ := json.Marshal(data)

	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
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
	case S.Events <- newMessage(message, stream):
	case <-time.After(time.Second):
		http.Error(w, "Timed out creating message", 504)
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

func (s *Server) Save(message *Message) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	// check the listing thing
	stream, ok := s.streams[message.Stream]
	if !ok {
		stream = newStream(message.Stream, false, int(streamTTL.Seconds()))
		s.streams[stream.Id] = stream
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
			s.Save(message)
			go s.Metadata(message)
		case <-t1.C:
			now := time.Now().UnixNano()

			s.mtx.Lock()

			// delete the metadata
			for metadata, g := range s.metadata {
				if d := now - g.Created; d > streamTTL.Nanoseconds() {
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
				}
			}

			s.mtx.Unlock()
		}
	}
}

func main() {
	go S.Run()

	htmlContent, err := fs.Sub(html, "html")
	if err != nil {
		log.Fatal(err)
	}

	// serve the html directory by default
	http.Handle("/", http.FileServer(http.FS(htmlContent)))

	http.HandleFunc("/new", func(w http.ResponseWriter, r *http.Request) {
		f, err := htmlContent.Open("new.html")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		b, err := ioutil.ReadAll(f)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write(b)
	})

	http.HandleFunc("/streams", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			getStreamsHandler(w, r)
		case "POST":
			newStreamHandler(w, r)
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
