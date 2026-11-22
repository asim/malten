package main

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/asim/malten/agent"
	"github.com/asim/malten/server"
)

//go:embed client/web/*
var html embed.FS

func main() {
	if err := server.Default.Restore("streams.json"); err != nil {
		log.Fatal(err)
	}
	http.Handle("/mcp", server.MCPHandler(server.Default))
	htmlContent, err := fs.Sub(html, "client/web")
	if err != nil {
		log.Fatal(err)
	}

	if err := mime.AddExtensionType(".webmanifest", "application/manifest+json"); err != nil {
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

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// run the server
	go server.Run()

	// run the agent
	agent.Run(ctx)

	log.Print("Listening on :9090")

	httpServer := &http.Server{Addr: ":9090", Handler: h}
	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		timeout, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if err := httpServer.Shutdown(timeout); err != nil {
			log.Print(err)
		}
		close(done)
	}()
	if err := httpServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	<-done
	if err := server.Default.Checkpoint(); err != nil {
		log.Fatal(err)
	}
}
