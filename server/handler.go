package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"malten.ai/agent"
	"malten.ai/command"
	"malten.ai/spatial"
)

// generateCommandID creates a unique command ID for async tracking
func generateCommandID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return "cmd_" + hex.EncodeToString(b)
}

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
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(command.GetMeta())
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
		Stream:  stream,
		Input:   input,
	}
	// First try location from POST (inline with command)
	var locUpdate *command.LocationUpdate
	if latStr := r.Form.Get("lat"); latStr != "" {
		if lat, err := strconv.ParseFloat(latStr, 64); err == nil {
			if lon, err := strconv.ParseFloat(r.Form.Get("lon"), 64); err == nil {
				ctx.Lat = lat
				ctx.Lon = lon
				// GPS accuracy
				if accStr := r.Form.Get("accuracy"); accStr != "" {
					if acc, err := strconv.ParseFloat(accStr, 64); err == nil {
						ctx.Accuracy = acc
					}
				}
				// Store location and check for prompts/notifications
				locUpdate = command.SetLocation(token, lat, lon)
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

	// Check for async mode
	asyncMode := r.Form.Get("async") == "true"

	if asyncMode {
		// Async mode: return immediately, push result via WebSocket
		cmdID := generateCommandID()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"id":     cmdID,
			"status": "queued",
		})

		// Process command in background
		go func() {
			if result, handled := command.Dispatch(ctx); handled {
				if result != "" {
					// Push result with command ID
					log.Printf("[async] Pushing command_result to stream=%s channel=@%s len=%d", stream, token[:8], len(result))
					Default.Events <- NewCommandResult(cmdID, result, stream, "@"+token)
				}
				// Push any context change messages
				for _, msg := range ctx.PushMessages {
					Default.Events <- NewChannelMessage(msg, stream, "@"+token)
				}
			} else {
				// AI handling (async mode) - get result and send as command_result
				result := handleAI(input, stream, token, true) // sync=true to get the result back
				if result != "" {
					Default.Events <- NewCommandResult(cmdID, result, stream, "@"+token)
				}
			}
			// Handle location-based prompts
			if locUpdate != nil {
				if locUpdate.ShouldPromptCheckIn {
					sendCheckInPrompt(token, stream, ctx.Lat, ctx.Lon)
				}
				if locUpdate.ArrivedAt != "" {
					sendArrivalPrompt(token, stream, locUpdate.ArrivedAt, ctx.Lat, ctx.Lon)
				}
				if locUpdate.PassingBy != "" {
					sendPassingPrompt(token, stream, locUpdate.PassingBy)
				}
			}
		}()
		return
	}

	// Sync mode (default): wait for result
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
		// Check if we should prompt for check-in (GPS stuck) or notify arrival
		if locUpdate != nil {
			if locUpdate.ShouldPromptCheckIn {
				go sendCheckInPrompt(token, stream, ctx.Lat, ctx.Lon)
			}
			if locUpdate.ArrivedAt != "" {
				go sendArrivalPrompt(token, stream, locUpdate.ArrivedAt, ctx.Lat, ctx.Lon)
			}
			if locUpdate.PassingBy != "" {
				go sendPassingPrompt(token, stream, locUpdate.PassingBy)
			}
		}
		return
	}

	// Everything else goes to AI with tool selection
	// Sync mode: wait for AI response and return it
	reply := handleAI(input, stream, token, true)
	if reply != "" {
		w.Write([]byte(reply))
	}
}

// handleAI processes AI queries. If sync is true, returns the response string.
// If sync is false, sends response via WebSocket and returns empty string.
func handleAI(prompt, stream, token string, sync bool) string {
	if agent.Client == nil {
		if sync {
			return "AI not available"
		}
		Default.Events <- NewChannelMessage("AI not available", stream, "@"+token)
		return ""
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
		if sync {
			return "Sorry, I encountered an error processing your request."
		}
		return ""
	}

	if sync {
		// Sync mode: return the response directly
		log.Printf("[handleAI] Returning sync reply for stream=%s", stream)
		return reply
	}

	// Async mode: Send to session's channel
	log.Printf("[handleAI] Sending reply to stream=%s channel=@%s", stream, token)
	Default.Events <- NewChannelMessage(reply, stream, "@"+token)
	return ""
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

// DebugHandler returns server debug info (memory, entities, uptime)
func DebugHandler(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	db := spatial.Get()
	stats := db.Stats()
	cacheStats := spatial.GetCacheStats().CacheSummary()

	data := map[string]interface{}{
		"memory": map[string]interface{}{
			"alloc_mb": float64(m.Alloc) / 1024 / 1024,
			"sys_mb":   float64(m.Sys) / 1024 / 1024,
			"gc":       m.NumGC,
		},
		"entities": map[string]interface{}{
			"total":    stats.Total,
			"agents":   stats.Agents,
			"weather":  stats.Weather,
			"prayer":   stats.Prayer,
			"arrivals": stats.Arrivals,
			"places":   stats.Places,
		},
		"cache":  cacheStats,
		"uptime": time.Since(startTime).Round(time.Second).String(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

var startTime = time.Now()

// BackfillStreetsHandler triggers street backfill from a center point
func BackfillStreetsHandler(w http.ResponseWriter, r *http.Request) {
	// Hampton coordinates as default
	lat := 51.4179
	lon := -0.3706
	maxRadius := 5000.0 // 5km default
	
	if r.URL.Query().Get("lat") != "" {
		fmt.Sscanf(r.URL.Query().Get("lat"), "%f", &lat)
	}
	if r.URL.Query().Get("lon") != "" {
		fmt.Sscanf(r.URL.Query().Get("lon"), "%f", &lon)
	}
	if r.URL.Query().Get("radius") != "" {
		fmt.Sscanf(r.URL.Query().Get("radius"), "%f", &maxRadius)
	}
	
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "Starting street backfill from %.4f,%.4f radius %.0fm\n", lat, lon, maxRadius)
	w.(http.Flusher).Flush()
	
	// Run in background
	go func() {
		count := spatial.BackfillStreetsForAllPlaces(lat, lon, maxRadius)
		log.Printf("[backfill] Complete: %d streets added", count)
	}()
	
	fmt.Fprintf(w, "Backfill started in background. Check logs for progress.\n")
}
