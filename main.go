package main

import (
	"embed"
	"flag"
	"io/fs"
	"log"
	"net/http"

	"github.com/asim/malten/agent"
	"github.com/asim/malten/server"
)

//go:embed client/web/*
var html embed.FS

var webDir = flag.String("web", "", "Serve static files from this directory (dev mode)")

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

	// serve static files
	http.Handle("/", http.FileServer(staticFS))

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
