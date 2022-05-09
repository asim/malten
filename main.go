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
	maxThoughtSize = 512
	maxThoughts    = 1000
	maxStreams     = 1000
	streamTTL      = 8.64e13
)

type Glimmer struct {
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
	Thoughts []*Thought
	Updated  int64
}

type Thought struct {
	Id      string
	Text    string
	Created int64 `json:",string"`
	Stream  string
	Glimmer *Glimmer
}

type Consciousness struct {
	Created int64
	Updates chan *Thought

	mtx      sync.RWMutex
	Streams  *lru.Cache
	streams  map[string]int64
	glimmers map[string]*Glimmer
}

//go:embed html/*
var html embed.FS

var (
	C = newConsciousness()
)

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
}

func newConsciousness() *Consciousness {
	return &Consciousness{
		Created:  time.Now().UnixNano(),
		Streams:  lru.New(maxStreams),
		Updates:  make(chan *Thought, 100),
		streams:  make(map[string]int64),
		glimmers: make(map[string]*Glimmer),
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
		Id:      uuid.New().String(),
		Text:    text,
		Created: time.Now().UnixNano(),
		Stream:  stream,
	}
}

func getGlimmer(uri string) *Glimmer {
	u, err := url.Parse(uri)
	if err != nil {
		return nil
	}

	d, err := goquery.NewDocument(u.String())
	if err != nil {
		return nil
	}

	g := &Glimmer{
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
	thought := r.Form.Get("id")
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

	thoughts := C.Retrieve(thought, stream, direction, last, limit)
	b, _ := json.Marshal(thoughts)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, string(b))
}

func getStreamsHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	b, _ := json.Marshal(C.List())
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
	case <-time.After(time.Second):
		http.Error(w, "Timed out creating thought", 504)
	}
}

func (c *Consciousness) Glimmer(t *Thought) {
	parts := strings.Split(t.Text, " ")
	for _, part := range parts {
		g := getGlimmer(part)
		if g == nil {
			continue
		}
		c.mtx.Lock()
		c.glimmers[t.Id] = g
		c.mtx.Unlock()
		return
	}
}

func (c *Consciousness) List() map[string]int64 {
	c.mtx.RLock()
	streams := c.streams
	c.mtx.RUnlock()
	return streams
}

func (c *Consciousness) Save(thought *Thought) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

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

func (c *Consciousness) Retrieve(thought string, streem string, direction, last, limit int64) []*Thought {
	c.mtx.RLock()
	defer c.mtx.RUnlock()

	var stream *Stream

	if object, ok := c.Streams.Get(streem); ok {
		stream = object.(*Stream)
	} else {
		return []*Thought{}
	}

	if len(thought) == 0 {
		var thoughts []*Thought

		if limit <= 0 {
			return thoughts
		}

		li := int(limit)

		// go back in time
		if direction < 0 {
			for i := len(stream.Thoughts) - 1; i >= 0; i-- {
				if len(thoughts) >= li {
					return thoughts
				}

				thought := stream.Thoughts[i]

				if thought.Created < last {
					if g, ok := c.glimmers[thought.Id]; ok {
						tc := *thought
						tc.Glimmer = g
						thoughts = append(thoughts, &tc)
					} else {
						thoughts = append(thoughts, thought)
					}
				}
			}
			return thoughts
		}

		start := 0
		if len(stream.Thoughts) > li {
			start = len(stream.Thoughts) - li
		}

		for i := start; i < len(stream.Thoughts); i++ {
			if len(thoughts) >= li {
				return thoughts
			}

			thought := stream.Thoughts[i]

			if thought.Created > last {
				if g, ok := c.glimmers[thought.Id]; ok {
					tc := *thought
					tc.Glimmer = g
					thoughts = append(thoughts, &tc)
				} else {
					thoughts = append(thoughts, thought)
				}
			}
		}
		return thoughts
	}

	// retrieve one
	for _, t := range stream.Thoughts {
		var thoughts []*Thought
		if thought == t.Id {
			if g, ok := c.glimmers[t.Id]; ok {
				tc := *t
				tc.Glimmer = g
				thoughts = append(thoughts, &tc)
			} else {
				thoughts = append(thoughts, t)
			}
			return thoughts
		}
	}

	return []*Thought{}
}

func (c *Consciousness) Think() {
	t1 := time.NewTicker(time.Hour)
	t2 := time.NewTicker(time.Minute)
	streams := make(map[string]int64)

	for {
		select {
		case thought := <-c.Updates:
			c.Save(thought)
			streams[thought.Stream] = time.Now().UnixNano()
			go c.Glimmer(thought)
		case <-t1.C:
			now := time.Now().UnixNano()
			for stream, u := range streams {
				if d := now - u; d > streamTTL {
					c.Streams.Remove(stream)
					delete(streams, stream)
				}
			}
			c.mtx.Lock()
			for glimmer, g := range c.glimmers {
				if d := now - g.Created; d > streamTTL {
					delete(c.glimmers, glimmer)
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
	go C.Think()

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
