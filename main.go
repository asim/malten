package main

import (
	"context"
	"embed"
	"io/fs"
	//"io/ioutil"
	"log"
	"net/http"

	"github.com/asim/malten/agent"
	"github.com/asim/malten/client/whatsapp"
	"github.com/asim/malten/server"
)

//go:embed client/web/*
var html embed.FS

func main() {
	htmlContent, err := fs.Sub(html, "client/web")
	if err != nil {
		log.Fatal(err)
	}

	// serve the html directory by default
	http.Handle("/", http.FileServer(http.FS(htmlContent)))

	http.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		server.GetEvents(w, r)
	})

	http.HandleFunc("/new", func(w http.ResponseWriter, r *http.Request) {
		// generate a new stream
		stream := server.Random(8)
		private := true
		ttl := 60
		// secret := TODO

		if err := server.Default.New(stream, "", private, ttl); err != nil {
			http.Error(w, "Cannot create stream", 500)
			return
		}

		// redirect to the stream
		http.Redirect(w, r, "/#"+stream, 302)

		/*
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
		*/
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// run the server
	go server.Run()

	// run the agent
	go agent.Run()

	// whatsapp client
	go whatsapp.Run(ctx)

	log.Print("Listening on :9090")

	if err := http.ListenAndServe(":9090", h); err != nil {
		log.Fatal(err)
	}
}
