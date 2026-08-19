// Command malten runs the spatial-exploration app as a single self-contained
// HTTP server with the UI (and a vendored map library) baked in.
//
// Configuration (environment variables):
//
//	MALTEN_ADDR    listen address (default :8080)
//
// The server is stateless: your finds and your Ordnance Survey API key live in
// the browser. Map tiles are fetched by the client directly from the OS APIs.
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
	log.Printf("malten listening on %s (spatial exploration, stateless)", addr)
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
