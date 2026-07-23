// Command malten runs the support agent as a single self-contained HTTP server
// with the chat UI baked in.
//
// Configuration (environment variables):
//
//	MALTEN_ADDR    listen address (default :8080)
//	MALTEN_DB      SQLite file path (default malten.db)
//	MALTEN_LLM     "stub" (default) or "claude"
//	MALTEN_MODEL   Claude model id (default claude-opus-4-8) when MALTEN_LLM=claude
//	ANTHROPIC_API_KEY  used by the claude backend
//
// With no API key the stub backend runs so the app is fully usable offline.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/asim/malten/internal/app"
	"github.com/asim/malten/internal/llm"
	"github.com/asim/malten/internal/server"
)

func main() {
	addr := env("MALTEN_ADDR", ":8080")
	dbPath := env("MALTEN_DB", "malten.db")

	model := chooseModel()

	ag, st, err := app.Build(model, dbPath)
	if err != nil {
		log.Fatalf("build: %v", err)
	}
	defer st.Close()

	srv := server.New(ag, st)
	log.Printf("malten listening on %s (model=%s, db=%s)", addr, model.Name(), dbPath)
	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}

func chooseModel() llm.LLM {
	backend := env("MALTEN_LLM", "")
	if backend == "" {
		// Auto: use Claude if an API key is present, else the stub.
		if os.Getenv("ANTHROPIC_API_KEY") != "" {
			backend = "claude"
		} else {
			backend = "stub"
		}
	}
	if backend == "claude" {
		return llm.NewClaude(env("MALTEN_MODEL", ""))
	}
	return llm.NewStub()
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
