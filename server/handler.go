package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"malten.ai/agent"
	"malten.ai/command"
)

var (
	defaultStream = "~"
)

// JsonError writes a JSON error response
func JsonError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

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

	// Don't broadcast user input - only broadcast responses
	// The HTTP response goes directly to the client
	// WebSocket is for server-initiated messages (context changes, AI responses, etc)


	// Build context for command dispatch
	ctx := &command.Context{
		Session: token,
		Input:   input,
	}
	// First try location from POST (inline with command)
	var shouldPromptCheckIn bool
	if latStr := r.Form.Get("lat"); latStr != "" {
		if lat, err := strconv.ParseFloat(latStr, 64); err == nil {
			if lon, err := strconv.ParseFloat(r.Form.Get("lon"), 64); err == nil {
				ctx.Lat = lat
				ctx.Lon = lon
				// Store location and check if GPS is stuck
				shouldPromptCheckIn = command.SetLocation(token, lat, lon)
			}
		}
	}
	// Destination coordinates (for directions)
	if toLatStr := r.Form.Get("toLat"); toLatStr != "" {
		if toLat, err := strconv.ParseFloat(toLatStr, 64); err == nil {
			if toLon, err := strconv.ParseFloat(r.Form.Get("toLon"), 64); err == nil {
				ctx.ToLat = toLat
				ctx.ToLon = toLon
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
			w.Write([]byte(result))
		}
		// Push any context change messages to the user's channel
		if len(ctx.PushMessages) > 0 {
			for _, msg := range ctx.PushMessages {
				Default.Events <- NewChannelMessage(msg, stream, "@"+token)
			}
		}
		// Check if we should prompt for check-in (GPS stuck)
		if shouldPromptCheckIn {
			go sendCheckInPrompt(token, stream, ctx.Lat, ctx.Lon)
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

	// Build system prompt with user's location context
	systemPrompt := agent.DefaultPrompt
	if userCtx := command.GetUserContext(token); userCtx != "" {
		systemPrompt += "\n\nUser's current location context:\n" + userCtx
		log.Printf("[AI] Context for %s: %d chars", token, len(userCtx))
	} else {
		log.Printf("[AI] No context for token %s", token)
	}

	reply, err := agent.Prompt(systemPrompt, ctx, prompt)
	if err != nil {
		fmt.Println("AI error:", err)
		return
	}

	// Send to session's channel
	log.Printf("[handleAI] Sending reply to stream=%s channel=@%s", stream, token)
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

	log.Printf("[events] New observer: stream=%s session=%s", stream, session)
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
		JsonError(w, "cannot create stream", 500)
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
		JsonError(w, "message required", 400)
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
		JsonError(w, "timed out", 504)
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
