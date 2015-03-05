package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"code.google.com/p/go-uuid/uuid"
	"github.com/golang/groupcache/lru"
)

const (
	defaultStream  = "_"
	maxThoughtSize = 512
	maxThoughts    = 1000
	maxStreams     = 1000
	streamTTL      = 86400
)

type Request struct {
	Last     int64
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
	Created int64 `json:",string"`
	Stream  string
}

type Consciousness struct {
	Created  int64
	Streams  *lru.Cache
	Requests chan *Request
	Updates  chan *Thought
}

var (
	C = newConsciousness()
)

func newConsciousness() *Consciousness {
	return &Consciousness{
		Created:  time.Now().UnixNano(),
		Streams:  lru.New(maxStreams),
		Requests: make(chan *Request, 100),
		Updates:  make(chan *Thought, 100),
	}
}

func newRequest(thought, stream string, last int64) *Request {
	return &Request{
		Last:     last,
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

	last, err := strconv.ParseInt(r.Form.Get("last"), 10, 64)
	if err != nil {
		last = 0
	}

	// default stream
	if len(stream) == 0 {
		stream = defaultStream
	}

	req := newRequest(thought, stream, last)
	C.Requests <- req
	thoughts := <-req.Response

	b, _ := json.Marshal(thoughts)
	log.Printf("%+v %d %+v", thoughts, last, string(b))
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, string(b))
}

func postHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	thought := r.Form.Get("text")
	stream := r.Form.Get("stream")

	if len(thought) == 0 {
		http.Error(w, "Thought cannot be blank", 400)
		return
	}

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
	var stream *Stream

	if object, ok := c.Streams.Get(thought.Stream); ok {
		stream = object.(*Stream)
	} else {
		stream = newStream(thought.Stream)
		c.Streams.Add(thought.Stream, stream)
	}

	stream.Thoughts = append(stream.Thoughts, thought)
	if len(stream.Thoughts) > maxThoughts {
		stream.Thoughts = stream.Thoughts[1:]
	}
	stream.Updated = time.Now().UnixNano()
}

func (c *Consciousness) Retrieve(thought string, streem string, last int64) []*Thought {
	var stream *Stream

	if object, ok := c.Streams.Get(streem); ok {
		stream = object.(*Stream)
	} else {
		return []*Thought{}
	}

	// retrieve all
	if len(thought) == 0 && last <= 0 {
		return stream.Thoughts
	}

	// retrieve delta
	if len(thought) == 0 && last > 0 {
		var thoughts []*Thought
		for _, thought := range stream.Thoughts {
			if thought.Created > last {
				thoughts = append(thoughts, thought)
			}
		}
		return thoughts
	}

	// retrieve one
	for _, t := range stream.Thoughts {
		if thought == t.Id {
			return []*Thought{t}
		}
	}

	return []*Thought{}
}

func (c *Consciousness) Think() {
	tick := time.NewTicker(time.Hour)
	streams := make(map[string]int64)

	for {
		select {
		case thought := <-c.Updates:
			c.Save(thought)
			streams[thought.Stream] = time.Now().Unix()
		case req := <-c.Requests:
			req.Response <- c.Retrieve(req.Thought, req.Stream, req.Last)
		case <-tick.C:
			now := time.Now().Unix()
			for stream, u := range streams {
				if d := now - u; d > streamTTL {
					c.Streams.Remove(stream)
					delete(streams, stream)
				}
			}
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
