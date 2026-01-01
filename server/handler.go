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
	input := r.Form.Get("prompt")
	token := getSessionToken(w, r)
	if len(input) == 0 {
		return
	}

	// default stream
	if len(stream) == 0 {
		stream = defaultStream
	}

	// default length
	if len(input) > MaxMessageSize {
		input = input[:MaxMessageSize]
	}

	// Save user message to their session channel (private)
	select {
	case Default.Events <- NewChannelMessage(input, stream, "@"+token):
	case <-time.After(time.Second):
		http.Error(w, "Timed out creating message", 504)
		return
	}

	// Handle navigation commands without slash (goto, new)
	if cmd := detectNavCommand(input); cmd != "" {
		input = cmd
	}

	// Build context for command dispatch
	ctx := &command.Context{
		Session: token,
		Input:   input,
	}
	// First try location from POST (inline with command)
	if latStr := r.Form.Get("lat"); latStr != "" {
		if lat, err := strconv.ParseFloat(latStr, 64); err == nil {
			if lon, err := strconv.ParseFloat(r.Form.Get("lon"), 64); err == nil {
				ctx.Lat = lat
				ctx.Lon = lon
			}
		}
	}
	// Fall back to session location
	if ctx.Lat == 0 && ctx.Lon == 0 {
		if loc := command.GetLocation(token); loc != nil {
			ctx.Lat = loc.Lat
			ctx.Lon = loc.Lon
		}
	}

	// Try command dispatch (handles /commands and natural language)
	if result, handled := command.Dispatch(ctx); handled {
		if result != "" {
			// Return response directly (HTTP) and broadcast to session channel (WebSocket)
			w.Write([]byte(result))
			Default.Events <- NewChannelMessage(result, stream, "@"+token)
		}
		return
	}

	// Everything else goes to AI with tool selection
	// Response goes to session channel
	go handleAI(input, stream, token)
}

// sendToSession sends a message to the user's private channel
func sendToSession(text, stream, token string) {
	Default.Events <- NewChannelMessage(text, stream, "@"+token)
}

func handleCommand(cmd, stream, token string) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return
	}

	name := strings.ToLower(strings.TrimPrefix(parts[0], "/"))
	args := parts[1:]

	// Check pluggable commands first
	if result, err := command.Execute(name, args); result != "" || err != nil {
		if err != nil {
			sendToSession("Error: "+err.Error(), stream, token)
		} else {
			sendToSession(result, stream, token)
		}
		return
	}

	// Built-in commands
	switch name {
	case "help", "commands":
		help := `/help - Show this help
/new - Create a new stream
/goto <stream> - Switch to a stream
/ping on|off - Enable/disable location sharing
/nearby <type> - Find nearby places (cafes, restaurants, etc)
/bus - Live bus arrivals nearby
/price <coin> - Get crypto price
/reminder [query] - Daily reminder or search Islamic texts
/chat <question> - Ask AI with real-time context
/news [query] - Latest news or search
/video <query> - Search videos
/blog - Latest blog posts`
		sendToSession(help, stream, token)

	case "streams":
		var names []string
		for k, v := range Default.List() {
			if !v.Private {
				names = append(names, "#"+k)
			}
		}
		if len(names) == 0 {
			sendToSession("No public streams", stream, token)
		} else {
			sendToSession(strings.Join(names, "\n"), stream, token)
		}

	case "new":
		name := Random(8)
		if len(args) > 0 {
			name = args[0]
		}
		if err := Default.New(name, "", false, int(StreamTTL.Seconds())); err != nil {
			sendToSession("Failed to create stream", stream, token)
		} else {
			sendToSession("Created stream #"+name+" - click to join: #"+name, stream, token)
		}

	case "goto":
		if len(args) > 0 {
			name := strings.TrimPrefix(args[0], "#")
			sendToSession("Click to join: #"+name, stream, token)
		} else {
			sendToSession("Usage: goto <stream>", stream, token)
		}

	case "ping":
		sendToSession(HandlePingCommand(cmd, token), stream, token)

	case "nearby", "near":
		sendToSession(HandleNearbyCommand(args, token), stream, token)

	case "bus", "buses":
		sendToSession(command.HandleBusCommand(token), stream, token)

	case "agents":
		sendToSession(HandleAgentsCommand(), stream, token)
	}
}

// detectNavCommand checks if input is a navigation command (goto, new)
func detectNavCommand(input string) string {
	input = strings.TrimSpace(input)
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return ""
	}
	
	cmd := strings.ToLower(parts[0])
	// Only handle navigation commands client-side can't intercept
	if cmd == "goto" || cmd == "new" {
		return "/" + input
	}
	return ""
}

// detectWalkQuery extracts destination from walking queries
// Handles: "how long to walk to X", "walk to X", "walking time to X", "how far is X"
func detectWalkQuery(input string) string {
	input = strings.TrimSpace(input)
	lower := strings.ToLower(input)
	
	// Patterns to match
	patterns := []string{
		"how long to walk to ",
		"how far to walk to ",
		"walking time to ",
		"walk time to ",
		"walk to ",
		"how far is ",
		"how long to ",
	}
	
	for _, p := range patterns {
		if idx := strings.Index(lower, p); idx != -1 {
			dest := strings.TrimSpace(input[idx+len(p):])
			// Remove trailing question mark
			dest = strings.TrimSuffix(dest, "?")
			if dest != "" {
				return dest
			}
		}
	}
	return ""
}

// detectNearbyQuery checks if input is a nearby/location query
// Handles: "cafes near me", "nearby cafes", "Twickenham cafes", "petrol station"
func detectNearbyQuery(input string) (bool, []string) {
	input = strings.TrimSpace(input)
	lower := strings.ToLower(input)
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return false, nil
	}

	// Remove filler words
	var cleaned []string
	for _, p := range parts {
		l := strings.ToLower(p)
		if l == "near" || l == "nearby" || l == "me" || l == "in" || l == "around" {
			continue
		}
		cleaned = append(cleaned, p)
	}

	// Check for multi-word types first (e.g., "petrol station")
	multiType, remaining := command.CheckMultiWordType(cleaned)
	if multiType != "" {
		// Found multi-word type, return it with any remaining location words
		result := append([]string{multiType}, remaining...)
		return true, result
	}

	// Check if any word is a valid place type
	hasPlaceType := false
	for _, p := range cleaned {
		if command.IsValidPlaceType(strings.ToLower(p)) {
			hasPlaceType = true
			break
		}
	}

	if !hasPlaceType {
		return false, nil
	}

	// Check if it looks like a nearby query
	// Either starts with nearby/near, contains "near me", or is just "<location> <type>"
	if strings.HasPrefix(lower, "near") ||
		strings.Contains(lower, "near me") ||
		strings.Contains(lower, "around me") ||
		len(cleaned) >= 1 {
		return true, cleaned
	}

	return false, nil
}

func handleAI(prompt, stream, token string) {
	if agent.Client == nil {
		Default.Events <- NewChannelMessage("AI not available", stream, "@"+token)
		return
	}

	// Set token context for location lookups
	agent.CurrentStream = token

	// Get recent messages for context (from this session's channel)
	messages := Default.RetrieveForSession(stream, token, 0, 20)
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

	// Send to session's channel
	Default.Events <- NewChannelMessage(reply, stream, "@"+token)
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

	// default stream
	if len(stream) == 0 {
		stream = defaultStream
	}

	// Get session for channel filtering
	session := getSessionToken(w, r)

	// Get by ID or list filtered by session
	var messages []*Message
	if message != "" {
		messages = Default.Retrieve(message, stream, 1, 0, 1)
	} else {
		messages = Default.RetrieveForSession(stream, session, last, limit)
	}
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

	// Get session for channel filtering
	session := getSessionToken(w, r)

	o := NewObserver(stream, session)

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

	// Public post to stream (no channel = public)
	select {
	case Default.Events <- NewChannelMessage(message, stream, ""):
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
