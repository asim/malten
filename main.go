package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"

	"github.com/asim/malten/agent"
	"github.com/asim/malten/server"
)

//go:embed client/web/*
var html embed.FS

func main() {
	htmlContent, err := fs.Sub(html, "client/web")
	if err != nil {
		log.Fatal(err)
	}

	// Initialize AI agent
	if err := agent.Init(); err != nil {
		log.Printf("AI not available: %v", err)
	} else {
		log.Println("AI initialized")
	}

	// serve the html directory by default
	http.Handle("/", http.FileServer(http.FS(htmlContent)))

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
