package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"code.google.com/p/go-uuid/uuid"
)

const (
	defaultStream  = "_"
	maxThoughtSize = 512
	maxThoughts    = 1000
	maxStreams     = 1000
)

type Request struct {
	Stream   string
	Thought  string
	Response chan []*Thought
}

type Stream struct {
	Id       string
	Thoughts []*Thought
	Updated  int64
}

type Thought struct {
	Id      string
	Text    string
	Created int64
	Stream  string
}

type Consciousness struct {
	Created  int64
	Streams  map[string]*Stream
	Requests chan *Request
	Updates  chan *Thought
}

var (
	C = newConsciousness()
)

func newConsciousness() *Consciousness {
	return &Consciousness{
		Created:  time.Now().UnixNano(),
		Streams:  make(map[string]*Stream),
		Requests: make(chan *Request, 100),
		Updates:  make(chan *Thought, 100),
	}
}

func newRequest(thought, stream string) *Request {
	return &Request{
		Thought:  thought,
		Stream:   stream,
		Response: make(chan []*Thought, 1),
	}
}

func newStream(id string) *Stream {
	return &Stream{
		Id:      id,
		Updated: time.Now().UnixNano(),
	}
}

func newThought(text, stream string) *Thought {
	return &Thought{
		Id:      uuid.New(),
		Text:    text,
		Created: time.Now().UnixNano(),
		Stream:  stream,
	}
}

func getHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	thought := r.Form.Get("id")
	stream := r.Form.Get("stream")

	// default stream
	if len(stream) == 0 {
		stream = defaultStream
	}

	req := newRequest(thought, stream)
	C.Requests <- req
	thoughts := <-req.Response

	b, _ := json.Marshal(thoughts)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, string(b))
}

func postHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	thought := r.Form.Get("text")
	stream := r.Form.Get("stream")

	// default stream
	if len(stream) == 0 {
		stream = defaultStream
	}

	// default length
	if len(thought) > maxThoughtSize {
		thought = thought[:maxThoughtSize]
	}

	select {
	case C.Updates <- newThought(thought, stream):
		http.Redirect(w, r, r.Referer(), 302)
	case <-time.After(time.Second):
		http.Error(w, "Timed out creating thought", 504)
	}
}

func (c *Consciousness) Save(thought *Thought) {
	stream, ok := c.Streams[thought.Stream]
	if !ok {
		stream = newStream(thought.Stream)
		c.Streams[thought.Stream] = stream
	}
	stream.Thoughts = append(stream.Thoughts, thought)
	if len(stream.Thoughts) > maxThoughts {
		stream.Thoughts = stream.Thoughts[1:]
	}
	stream.Updated = time.Now().UnixNano()
}

func (c *Consciousness) Retrieve(thought string, streem string) []*Thought {
	stream, ok := c.Streams[streem]
	if !ok {
		return []*Thought{}
	}

	if len(thought) == 0 {
		return stream.Thoughts
	}

	for _, t := range stream.Thoughts {
		if thought == t.Id {
			return []*Thought{t}
		}
	}

	return []*Thought{}
}

func (c *Consciousness) Think() {
	for {
		select {
		case thought := <-c.Updates:
			c.Save(thought)
		case req := <-c.Requests:
			req.Response <- c.Retrieve(req.Thought, req.Stream)
		}
	}
}

func main() {
	go C.Think()

	http.Handle("/", http.FileServer(http.Dir("html")))

	http.HandleFunc("/thoughts", func(w http.ResponseWriter, r *http.Request) {
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
