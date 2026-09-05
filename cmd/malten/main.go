// Command malten runs the reflection app as a single self-contained HTTP
// server with the UI baked in.
//
// Configuration (environment variables):
//
//	MALTEN_ADDR    listen address (default :8080)
//
// The server is stateless. It has no copy of the user's spatial network.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/asim/malten/internal/server"
)

func main() {
	addr := env("MALTEN_ADDR", ":8080")
	srv := server.New()
	log.Printf("malten listening on %s", addr)
	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
