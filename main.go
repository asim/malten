package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"malten.ai/agent"
	"malten.ai/command"
	"malten.ai/server"
	"malten.ai/spatial"
)

//go:embed client/web/*
var html embed.FS

var webDir = flag.String("web", "", "Serve static files from this directory (dev mode)")

const goGetTemplate = `<!DOCTYPE html>
<html>
<head>
<meta name="go-import" content="malten.ai%s git https://github.com/asim/malten%s">
<meta name="go-source" content="malten.ai%s https://github.com/asim/malten%s https://github.com/asim/malten%s/tree/main{/dir} https://github.com/asim/malten%s/blob/main{/dir}/{file}#L{line}">
</head>
<body>go get malten.ai%s</body>
</html>`

var versionRegex = regexp.MustCompile(`\?v=\d+`)

func serveFileWithVersion(w http.ResponseWriter, r *http.Request, staticFS http.FileSystem, path, version, contentType string) {
	f, err := staticFS.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	
	data, err := io.ReadAll(f)
	if err != nil {
		http.Error(w, "Error reading file", 500)
		return
	}
	
	content := strings.ReplaceAll(string(data), "__VERSION__", version)
	
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-cache")
	w.Write([]byte(content))
}

func serveIndexWithVersion(w http.ResponseWriter, r *http.Request, staticFS http.FileSystem, jsVersion string) {
	f, err := staticFS.Open("/index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	
	data, err := io.ReadAll(f)
	if err != nil {
		http.Error(w, "Error reading file", 500)
		return
	}
	
	// Replace all ?v=XX with ?v={jsVersion}
	html := versionRegex.ReplaceAllString(string(data), "?v="+jsVersion)
	
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache") // Always revalidate index.html
	w.Write([]byte(html))
}

func handleGoGet(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	
	// Determine the subpackage path
	var subPkg, repoSuffix string
	if path == "/" || path == "" {
		subPkg = ""
		repoSuffix = ""
	} else {
		subPkg = path
		// For subpackages, the repo is still the root
		if strings.HasPrefix(path, "/") {
			repoSuffix = ""
		}
	}
	
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, goGetTemplate, subPkg, repoSuffix, subPkg, repoSuffix, repoSuffix, repoSuffix, subPkg)
}

// buildPushNotification creates a push notification for a user's location
func buildPushNotification(lat, lon float64) *server.PushNotification {
	// Get fresh context data
	ctx := spatial.GetContextData(lat, lon)
	if ctx == nil {
		return nil
	}

	// Build notification based on what's relevant
	// Priority: bus times > prayer approaching > weather warning

	// Check for bus times
	if ctx.Bus != nil && len(ctx.Bus.Arrivals) > 0 {
		// Arrivals are strings like "185 → Victoria in 3m"
		return &server.PushNotification{
			Title: "🚌 " + ctx.Bus.StopName,
			Body:  ctx.Bus.Arrivals[0],
			Tag:   "bus",
		}
	}

	// Check for prayer approaching
	if ctx.Prayer != nil && ctx.Prayer.NextTime != "" {
		return &server.PushNotification{
			Title: "🕌 " + ctx.Prayer.Next,
			Body:  "at " + ctx.Prayer.NextTime,
			Tag:   "prayer",
		}
	}

	// Check for rain warning
	if ctx.Weather != nil && ctx.Weather.RainWarning != "" {
		return &server.PushNotification{
			Title: "🌧️ Rain",
			Body:  ctx.Weather.RainWarning,
			Tag:   "weather",
		}
	}

	return nil
}

// buildWeatherNotification creates morning weather notification
func buildWeatherNotification(lat, lon float64) *server.PushNotification {
	ctx := spatial.GetContextData(lat, lon)
	if ctx == nil || ctx.Weather == nil {
		return nil
	}

	body := ctx.Weather.Condition
	if ctx.Weather.RainWarning != "" {
		body += " · " + ctx.Weather.RainWarning
	}

	return &server.PushNotification{
		Title: "🌅 Good morning",
		Body:  body,
		Tag:   "morning",
	}
}

// buildPrayerNotification creates prayer reminder if prayer is ~10 min away
func buildPrayerNotification(lat, lon float64, now time.Time) *server.PushNotification {
	ctx := spatial.GetContextData(lat, lon)
	if ctx == nil || ctx.Prayer == nil || ctx.Prayer.NextTime == "" {
		return nil
	}

	// Parse next prayer time (format: "HH:MM")
	prayerTime, err := time.Parse("15:04", ctx.Prayer.NextTime)
	if err != nil {
		return nil
	}

	// Set prayer time to today
	prayerTime = time.Date(now.Year(), now.Month(), now.Day(), 
		prayerTime.Hour(), prayerTime.Minute(), 0, 0, now.Location())

	// Check if prayer is 8-12 minutes away (window for "10 min" reminder)
	untilPrayer := prayerTime.Sub(now)
	if untilPrayer >= 8*time.Minute && untilPrayer <= 12*time.Minute {
		return &server.PushNotification{
			Title: "🕌 " + ctx.Prayer.Next + " soon",
			Body:  "in about 10 minutes (" + ctx.Prayer.NextTime + ")",
			Tag:   "prayer-" + strings.ToLower(ctx.Prayer.Next),
		}
	}

	return nil
}

func main() {
	flag.Parse()
	
	var staticFS http.FileSystem
	if *webDir != "" {
		// Dev mode: serve from disk
		log.Printf("Serving static files from %s", *webDir)
		staticFS = http.Dir(*webDir)
	} else {
		// Production: use embedded files
		htmlContent, err := fs.Sub(html, "client/web")
		if err != nil {
			log.Fatal(err)
		}
		staticFS = http.FS(htmlContent)
	}

	// Initialize spatial DB (triggers agent recovery)
	spatial.Get()

	// Initialize push notifications
	pm := server.GetPushManager()
	command.UpdatePushLocation = pm.UpdateLocation
	server.SetNotificationBuilder(buildPushNotification)
	server.SetWeatherNotificationBuilder(buildWeatherNotification)
	server.SetPrayerNotificationBuilder(buildPrayerNotification)

	// Initialize AI agent
	if err := agent.Init(); err != nil {
		log.Printf("AI not available: %v", err)
	} else {
		log.Println("AI initialized")
	}

	// Get JS version from file mtime (for cache busting)
	var jsVersion string
	if *webDir != "" {
		if info, err := os.Stat(*webDir + "/malten.js"); err == nil {
			jsVersion = fmt.Sprintf("%d", info.ModTime().Unix())
		}
	} else {
		// Embedded: use build time
		jsVersion = fmt.Sprintf("%d", time.Now().Unix())
	}
	log.Printf("JS version: %s", jsVersion)

	// serve static files with go-get support and auto-versioning
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Handle go-get vanity imports for malten.ai
		if r.URL.Query().Get("go-get") == "1" {
			handleGoGet(w, r)
			return
		}
		
		path := r.URL.Path
		
		// For index.html, inject current JS version
		if path == "/" || path == "/index.html" {
			serveIndexWithVersion(w, r, staticFS, jsVersion)
			return
		}
		
		// For sw.js, inject version into cache name
		if path == "/sw.js" {
			serveFileWithVersion(w, r, staticFS, "/sw.js", jsVersion, "application/javascript")
			return
		}
		
		// No caching for HTML/JS/CSS - PWA caching caused too many issues
		if strings.HasSuffix(path, ".js") || strings.HasSuffix(path, ".css") || strings.HasSuffix(path, ".html") || path == "/" {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
		}
		
		http.FileServer(staticFS).ServeHTTP(w, r)
	})

	http.HandleFunc("/events", server.GetEvents)
	http.HandleFunc("/agents", server.AgentsHandler)
	http.HandleFunc("/agents/", server.AgentsHandler)
	
	// Command metadata for client
	http.HandleFunc("/commands/meta", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(command.GetMeta())
	})
	
	http.HandleFunc("/commands", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			server.GetCommandsHandler(w, r)
		case "POST":
			server.PostCommandHandler(w, r)
		default:
			server.JsonError(w, "method not allowed", 405)
		}
	})

	http.HandleFunc("/streams", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			// If WebSocket upgrade, stream events (like /events)
			if server.IsWebSocket(r) {
				server.GetEvents(w, r)
				return
			}
			server.GetStreamsHandler(w, r)
		case "POST":
			// POST to stream = send command (like /commands)
			server.PostCommandHandler(w, r)
		default:
			server.JsonError(w, "method not allowed", 405)
		}
	})

	http.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			server.GetHandler(w, r)
		case "POST":
			server.PostHandler(w, r)
		default:
			server.JsonError(w, "method not allowed", 405)
		}
	})

	// Push notification endpoints
	http.HandleFunc("/push/subscribe", server.HandleSubscribe)
	http.HandleFunc("/push/unsubscribe", server.HandleUnsubscribe)
	http.HandleFunc("/push/vapid-key", server.HandleVAPIDKey)

	h := server.WithCors(http.DefaultServeMux)

	// run the server event loop
	go server.Run()

	log.Print("Listening on :9090")

	if err := http.ListenAndServe(":9090", h); err != nil {
		log.Fatal(err)
	}
}
