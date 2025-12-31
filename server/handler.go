package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/asim/malten/agent"
	"github.com/asim/malten/command"
)

var (
	defaultStream = "~"
)

func GetCommandsHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	// return list of commands
	help := `Available commands:
/help - Show this help
/commands - Show this help
/streams - List public streams
/new - Create a new stream
/goto <stream> - Switch to a stream`

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(help))
}

func PostCommandHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	stream := r.Form.Get("stream")
	command := r.Form.Get("prompt")
	if len(command) == 0 {
		return
	}

	// default stream
	if len(stream) == 0 {
		stream = defaultStream
	}

	// default length
	if len(command) > MaxMessageSize {
		command = command[:MaxMessageSize]
	}

	// Save user message first
	select {
	case Default.Events <- NewMessage(command, stream):
	case <-time.After(time.Second):
		http.Error(w, "Timed out creating message", 504)
		return
	}

	// Handle slash commands
	if strings.HasPrefix(command, "/") {
		handleCommand(command, stream)
		return
	}

	// Check if it's a price query
	if coin := detectPriceQuery(command); coin != "" {
		go handlePriceQuery(coin, stream)
		return
	}

	// Send to AI
	go handleAI(command, stream)
}

func handleCommand(cmd, stream string) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return
	}

	name := strings.ToLower(strings.TrimPrefix(parts[0], "/"))
	args := parts[1:]

	// Check pluggable commands first
	if result, err := command.Execute(name, args); result != "" || err != nil {
		if err != nil {
			Default.Events <- NewMessage("Error: "+err.Error(), stream)
		} else {
			Default.Events <- NewMessage(result, stream)
		}
		return
	}

	// Built-in commands
	switch name {
	case "help", "commands":
		help := `/help - Show this help
/streams - List public streams
/new - Create a new stream
/goto <stream> - Switch to a stream
/price <coin> - Get crypto price
/reminder - Daily Islamic reminder
/quran <query> - Search the Quran`
		Default.Events <- NewMessage(help, stream)

	case "streams":
		var names []string
		for k, v := range Default.List() {
			if !v.Private {
				names = append(names, "#"+k)
			}
		}
		if len(names) == 0 {
			Default.Events <- NewMessage("No public streams", stream)
		} else {
			Default.Events <- NewMessage(strings.Join(names, "\n"), stream)
		}

	case "new":
		name := Random(8)
		if len(args) > 0 {
			name = args[0]
		}
		if err := Default.New(name, "", false, int(StreamTTL.Seconds())); err != nil {
			Default.Events <- NewMessage("Failed to create stream", stream)
		} else {
			Default.Events <- NewMessage("/goto #"+name, stream)
		}

	case "goto":
		if len(args) > 0 {
			Default.Events <- NewMessage("Use #"+args[0]+" in the URL to switch streams", stream)
		} else {
			Default.Events <- NewMessage("Usage: /goto <stream>", stream)
		}
	}
}

// detectPriceQuery checks if the input is asking for a crypto price
func detectPriceQuery(input string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	
	// Patterns: "btc price", "price of btc", "btc", "bitcoin price", etc.
	coins := []string{
		"btc", "bitcoin", "eth", "ethereum", "uni", "uniswap",
		"sol", "solana", "ada", "cardano", "dot", "polkadot",
		"matic", "polygon", "link", "chainlink", "avax", "avalanche",
		"atom", "cosmos", "xrp", "ripple", "doge", "dogecoin",
		"shib", "ltc", "litecoin", "xlm", "stellar",
		"arb", "arbitrum", "op", "optimism", "pepe",
	}
	
	for _, coin := range coins {
		// "btc price", "btc?", "btc"
		if input == coin || input == coin+"?" || input == coin+" price" || input == coin+" price?" {
			return coin
		}
		// "price of btc", "price btc"
		if input == "price of "+coin || input == "price "+coin {
			return coin
		}
		// "what is btc", "what's btc" - only for ticker symbols
		if len(coin) <= 5 {
			if input == "what is "+coin || input == "whats "+coin || input == "what's "+coin {
				return coin
			}
		}
	}
	return ""
}

func handlePriceQuery(coin, stream string) {
	result, err := command.Execute("price", []string{coin})
	if err != nil {
		Default.Events <- NewMessage("Error: "+err.Error(), stream)
		return
	}
	Default.Events <- NewMessage(result, stream)
}

func handleAI(prompt, stream string) {
	if agent.Client == nil {
		Default.Events <- NewMessage("AI not available", stream)
		return
	}

	// Get recent messages for context
	messages := Default.Retrieve("", stream, 1, 0, 20)
	var ctx []agent.Message
	for _, m := range messages {
		if m.Type == "message" && m.Text != prompt {
			// Determine role based on content
			role := "user"
			if strings.HasPrefix(m.Text, "[malten]") {
				role = "assistant"
			}
			ctx = append(ctx, agent.Message{Role: role, Content: m.Text})
		}
	}

	reply, err := agent.Prompt(agent.DefaultPrompt, ctx, prompt)
	if err != nil {
		fmt.Println("AI error:", err)
		return
	}

	Default.Events <- NewMessage(reply, stream)
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
