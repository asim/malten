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
		
		// For JS/CSS, set cache headers
		if strings.HasSuffix(path, ".js") || strings.HasSuffix(path, ".css") {
			w.Header().Set("Cache-Control", "public, max-age=31536000") // 1 year (versioned URLs)
		}
		
		http.FileServer(staticFS).ServeHTTP(w, r)
	})

	http.HandleFunc("/events", server.GetEvents)
	http.HandleFunc("/agents", server.AgentsHandler)
	http.HandleFunc("/agents/", server.AgentsHandler)
	
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
			server.GetStreamsHandler(w, r)
		case "POST":
			server.NewStreamHandler(w, r)
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

	h := server.WithCors(http.DefaultServeMux)

	// run the server event loop
	go server.Run()

	log.Print("Listening on :9090")

	if err := http.ListenAndServe(":9090", h); err != nil {
		log.Fatal(err)
	}
}
