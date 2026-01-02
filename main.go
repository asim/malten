package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strings"

	"malten.ai/agent"
	"malten.ai/server"
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

	// Initialize AI agent
	if err := agent.Init(); err != nil {
		log.Printf("AI not available: %v", err)
	} else {
		log.Println("AI initialized")
	}

	// serve static files with go-get support
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Handle go-get vanity imports for malten.ai
		if r.URL.Query().Get("go-get") == "1" {
			handleGoGet(w, r)
			return
		}
		http.FileServer(staticFS).ServeHTTP(w, r)
	})

	http.HandleFunc("/events", server.GetEvents)

	http.HandleFunc("/ping", server.PingHandler)
	http.HandleFunc("/context", server.ContextHandler)
	http.HandleFunc("/place/", server.PlaceHandler)

	http.HandleFunc("/commands", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			server.GetCommandsHandler(w, r)
		case "POST":
			server.PostCommandHandler(w, r)
		default:
			http.Error(w, "unsupported method", 400)
		}
	})

	http.HandleFunc("/streams", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			server.GetStreamsHandler(w, r)
		case "POST":
			server.NewStreamHandler(w, r)
		default:
			http.Error(w, "unsupported method", 400)
		}
	})

	http.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			server.GetHandler(w, r)
		case "POST":
			server.PostHandler(w, r)
		default:
			http.Error(w, "unsupported method", 400)
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
