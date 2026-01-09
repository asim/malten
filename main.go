package main

import (
	"embed"
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
	"malten.ai/data"
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

// serveSW generates the service worker dynamically - no sw.js file needed
func serveSW(w http.ResponseWriter, version string) {
	sw := `// Malten Service Worker v` + version + ` - Push only, no caching
self.addEventListener('install', function(e) { self.skipWaiting(); });
self.addEventListener('activate', function(e) {
    e.waitUntil(
        caches.keys().then(function(names) {
            return Promise.all(names.map(function(name) { return caches.delete(name); }));
        }).then(function() { return clients.claim(); })
    );
});
self.addEventListener('push', function(e) {
    if (!e.data) return;
    var data;
    try { data = e.data.json(); } catch(err) { data = { title: 'Malten', body: e.data.text() }; }
    e.waitUntil(self.registration.showNotification(data.title || 'Malten', {
        body: data.body || '',
        icon: '/icon-192.png',
        badge: '/icon-192.png',
        image: data.image,
        tag: data.tag || 'malten',
        renotify: true,
        data: data.data || {}
    }));
});
self.addEventListener('notificationclick', function(e) {
    e.notification.close();
    e.waitUntil(clients.matchAll({type: 'window'}).then(function(list) {
        for (var i = 0; i < list.length; i++) if ('focus' in list[i]) return list[i].focus();
        if (clients.openWindow) return clients.openWindow('/');
    }));
});
`
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write([]byte(sw))
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
func buildPushNotification(lat, lon float64, busNotify bool) *server.PushNotification {
	// Get fresh context data
	ctx := spatial.GetContextData(lat, lon)
	if ctx == nil {
		return nil
	}

	// Build notification based on what's relevant
	// Priority: bus times (if enabled) > prayer approaching > weather warning

	// Check for bus times (only if user enabled bus notifications)
	if busNotify && ctx.Bus != nil && len(ctx.Bus.Arrivals) > 0 {
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

// buildMorningContext creates the 7am morning notification with weather and prayer times
func buildMorningContext(lat, lon float64) *server.PushNotification {
	ctx := spatial.GetContextData(lat, lon)
	if ctx == nil {
		return nil
	}

	title := "🌅 Good morning"
	var body string

	// Weather
	if ctx.Weather != nil {
		body = ctx.Weather.Condition
		if ctx.Weather.RainWarning != "" {
			body += " · " + ctx.Weather.RainWarning
		}
	}

	// Prayer times
	if ctx.Prayer != nil && ctx.Prayer.Display != "" {
		if body != "" {
			body += "\n"
		}
		body += ctx.Prayer.Display
	}

	if body == "" {
		return nil
	}

	image := spatial.FetchNatureImage("morning")

	return &server.PushNotification{
		Title: title,
		Body:  body,
		Image: image,
		Tag:   "morning",
	}
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

	// Initialize data files
	if err := data.Migrate("."); err != nil {
		log.Printf("Data migration warning: %v", err)
	}
	data.StartBackgroundSave(5 * time.Minute)
	log.Printf("[data] Loaded: %d subscriptions, %d notification sessions",
		len(data.Subscriptions().Users), len(data.Notifications().History))

	// Initialize spatial DB (triggers agent recovery)
	spatial.Get()

	// Start background cleanup every 5 minutes (only cleans expired arrivals)
	spatial.Get().StartBackgroundCleanup(5 * time.Minute)

	// Start courier loops (local and regional)
	spatial.StartCourierLoop()         // Original courier for backward compat
	spatial.StartRegionalCourierLoop() // Regional couriers for global coverage

	// Initialize push notifications (simple: just morning context at 7am)
	pm := server.GetPushManager()
	command.UpdatePushLocation = pm.UpdateLocation
	server.SetMorningContextBuilder(buildMorningContext)

	// Wire up callbacks for command package
	command.ResetSessionCallback = func(stream, session string) int {
		cleared := server.Default.ClearSessionChannel(stream, session)
		pm.Unsubscribe(session)
		return cleared
	}

	// Initialize AI agent
	if err := agent.Init(); err != nil {
		log.Printf("AI not available: %v", err)
	} else {
		log.Println("AI initialized")
		// Share client with dedupe system
		server.SetDedupeClient(agent.Client)
	}

	// Get JS version from malten.js VERSION constant
	var jsVersion string
	var jsContent []byte
	if *webDir != "" {
		jsContent, _ = os.ReadFile(*webDir + "/malten.js")
	} else {
		// Embedded
		if f, err := staticFS.Open("/malten.js"); err == nil {
			jsContent, _ = io.ReadAll(f)
			f.Close()
		}
	}
	// Parse VERSION from: var VERSION = 342;
	if matches := regexp.MustCompile(`var VERSION = (\d+);`).FindSubmatch(jsContent); len(matches) > 1 {
		jsVersion = string(matches[1])
	} else {
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

		// Generate sw.js dynamically (no separate file needed)
		if path == "/sw.js" {
			serveSW(w, jsVersion)
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
	http.HandleFunc("/debug", server.DebugHandler)
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
	http.HandleFunc("/push/history", server.HandlePushHistory)
	http.HandleFunc("/push/test-morning", server.HandleTestMorningPush)
	http.HandleFunc("/map", server.MapHandler)
	http.HandleFunc("/backfill-streets", server.BackfillStreetsHandler)

	h := server.WithCors(http.DefaultServeMux)

	// run the server event loop
	go server.Run()

	log.Print("Listening on :9090")

	if err := http.ListenAndServe(":9090", h); err != nil {
		log.Fatal(err)
	}
}
