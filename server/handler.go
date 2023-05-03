package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

var (
	defaultStream = "_"
)

func GetCommandsHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	// return list of commands
}

func PostCommandHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	stream := r.Form.Get("stream")
	command := r.Form.Get("prompt")
	if len(command) == 0 {
		return
	}

	fmt.Println("Got command", command)

	// default stream
	if len(stream) == 0 {
		stream = defaultStream
	}

	// default length
	if len(command) > MaxMessageSize {
		command = command[:MaxMessageSize]
	}

	select {
	case Default.Events <- NewCommand(command, stream):
	case <-time.After(time.Second):
		http.Error(w, "Timed out creating message", 504)
	}
}

func GetHandler(w http.ResponseWriter, r *http.Request) {
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

	messages := Default.Retrieve(message, stream, direction, last, limit)
	b, _ := json.Marshal(messages)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, string(b))
}

func GetEvents(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	stream := r.Form.Get("stream")
	// default stream
	if len(stream) == 0 {
		stream = defaultStream
	}

	o := NewObserver(stream)

	defer func() {
		close(o.Kill)
	}()

	// add self
	Default.Observe(o)

	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Transfer-Encoding", "chunked")

	// serve a socket
	if IsWebSocket(r) {
		ServeWebSocket(w, r, o)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")

	for {
		select {
		case message := <-o.Events:
			if message.Stream != o.Stream {
				fmt.Println("ignoring", message.Stream, o.Stream)
				continue
			}

			b, _ := json.Marshal(message)
			fmt.Fprintf(w, "data: %v\n\n", string(b))

			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		case <-r.Context().Done():
			return
		}
	}
}

func GetStreamsHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	streams := make(map[string]*Stream)

	for k, v := range Default.List() {
		// only return public streams
		if v.Private {
			continue
		}
		streams[k] = &Stream{
			Id:        v.Id,
			Updated:   v.Updated,
			TTL:       v.TTL,
			Observers: v.Observers,
		}
	}

	b, _ := json.Marshal(streams)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, string(b))
}

func NewStreamHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	stream := r.Form.Get("stream")
	private, _ := strconv.ParseBool(r.Form.Get("private"))
	ttl, _ := strconv.Atoi(r.Form.Get("ttl"))

	if len(stream) == 0 {
		stream = Random(8)
	}

	if ttl <= 0 {
		ttl = int(StreamTTL.Seconds())
	}

	if err := Default.New(stream, "", private, ttl); err != nil {
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

func PostHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	message := r.Form.Get("message")
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
	if len(message) > MaxMessageSize {
		message = message[:MaxMessageSize]
	}

	select {
	case Default.Events <- NewMessage(message, stream):
	case <-time.After(time.Second):
		http.Error(w, "Timed out creating message", 504)
	}
}

func WithCors(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// set cors origin allow all
		SetHeaders(w, r)

		// if options return immediately
		if r.Method == "OPTIONS" {
			return
		}

		h.ServeHTTP(w, r)
	})
}
