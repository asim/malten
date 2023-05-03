package main

import (
	"embed"
	"io/fs"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/asim/malten/agent/ai"
	"github.com/asim/malten/server"
)

//go:embed html/*
var html embed.FS

func main() {
	htmlContent, err := fs.Sub(html, "html")
	if err != nil {
		log.Fatal(err)
	}

	// serve the html directory by default
	http.Handle("/", http.FileServer(http.FS(htmlContent)))

	http.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		server.GetEvents(w, r)
	})

	http.HandleFunc("/new", func(w http.ResponseWriter, r *http.Request) {
		f, err := htmlContent.Open("new.html")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		b, err := ioutil.ReadAll(f)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write(b)
	})

	http.HandleFunc("/commands", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			server.GetCommandsHandler(w, r)
		case "POST":
			server.PostCommandHandler(w, r)
		default:
			http.Error(w, "unsupported method "+r.Method, 400)
		}
	})

	http.HandleFunc("/streams", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			server.GetStreamsHandler(w, r)
		case "POST":
			server.NewStreamHandler(w, r)
		default:
			http.Error(w, "unsupported method "+r.Method, 400)
		}
	})

	http.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			server.GetHandler(w, r)
		case "POST":
			server.PostHandler(w, r)
		default:
			http.Error(w, "unsupported method "+r.Method, 400)
		}
	})

	h := server.WithCors(http.DefaultServeMux)

	// run the server
	go server.Run()

	// run the ai
	go ai.Run()

	log.Print("Listening on :9090")

	if err := http.ListenAndServe(":9090", h); err != nil {
		log.Fatal(err)
	}
}
