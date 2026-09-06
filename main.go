// Command malten serves the stream and owns the agent lifecycle.
package main

import (
	"context"
	"errors"
	"github.com/asim/malten/agent"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/asim/malten/agent/aslam"
	"github.com/asim/malten/agent/nature"
	"github.com/asim/malten/agent/news"
	"github.com/asim/malten/agent/reminder"
	"github.com/asim/malten/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	path := os.Getenv("MALTEN_DATA")
	if path == "" {
		path = "data/stream.json"
	}
	srv, err := server.Open(path)
	if err != nil {
		log.Fatalf("open stream: %v", err)
	}
	var agents sync.WaitGroup
	start := func(run func(context.Context)) { agents.Add(1); go func() { defer agents.Done(); run(ctx) }() }
	memory, err := agent.Open(path + ".agents")
	if err != nil {
		log.Fatalf("open agent streams: %v", err)
	}
	srv.UseAgentContext(memory)
	srv.AgentStatus = func() any { return memory.Status() }
	start(srv.Run)
	for _, worker := range []agent.Agent{reminder.New(), aslam.New(), news.New(), nature.New()} {
		srv.AgentStreams = append(srv.AgentStreams, agent.Stream{Tag: worker.Name})
		observe := func() []agent.Observation {
			observations := srv.AgentObservations()
			for _, name := range []string{"reminder", "aslam", "news", "nature"} {
				if name == worker.Name {
					continue
				}
				records := memory.Read(name, time.Now())
				for i := len(records) - 1; i >= 0; i-- {
					r := records[i]
					if r.Kind == "source" {
						text := string(r.Data)
						if len(text) > 12000 {
							text = text[:12000]
						}
						observations = append(observations, agent.Observation{ID: r.ID, Stream: name, Text: text, Kind: "source", At: r.At})
						break
					}
				}
			}
			return observations
		}
		loop := agent.Loop{Agent: worker, Memory: memory, Observe: observe, Publish: srv.PublishAgentPhoto}
		start(loop.Run)
	}
	addr := os.Getenv("MALTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	httpServer := &http.Server{Addr: addr, Handler: srv.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 35 * time.Second, IdleTimeout: 60 * time.Second, BaseContext: func(_ net.Listener) context.Context { return ctx }}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdown)
	}()
	log.Printf("malten listening on %s", addr)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Print(err)
		stop()
	}
	stop()
	agents.Wait()
}
