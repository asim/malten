// Command malten serves the stream and owns the agent lifecycle.
package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/asim/malten/agent"
	"github.com/asim/malten/agent/aslam"
	"github.com/asim/malten/agent/daylight"
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
	start(srv.Run)
	srv.AgentStreams = append(srv.AgentStreams, reminder.Streams...)
	srv.AgentStreams = append(srv.AgentStreams, aslam.Streams...)
	srv.AgentStreams = append(srv.AgentStreams, news.Streams...)
	start(func(ctx context.Context) { reminder.Run(ctx, srv.PublishAgentPhoto) })
	start(func(ctx context.Context) { aslam.Run(ctx, srv.PublishAgentPhoto) })
	day := daylight.New()
	srv.AgentStreams = append(srv.AgentStreams, daylight.Streams...)
	start(func(ctx context.Context) { day.Run(ctx, srv.PublishAgentPhoto) })
	live := agent.NewLive(srv.RecentPosts, srv.PublishQuietAgent, aslam.Fetch, reminder.Fetch, day.Fetch)
	start(func(ctx context.Context) { news.Run(ctx, srv.PublishAgentPhoto) })
	live.Observe("home")
	for _, region := range agent.Regions {
		live.Observe(region.Tag)
	}
	for _, city := range agent.Cities {
		live.Observe(city.Tag)
	}
	srv.ObserveStream = live.Observe
	start(live.Run)
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
