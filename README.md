# Malten

A conversational agent that ships as a single Go binary — an embedded chat UI, a
bounded agent loop, a pluggable LLM backend, and a SQLite database, all in one
process with no external dependencies.

Right now it's wired as a **support agent**: you send a message, it answers,
takes an action, or hands off to a human. What exactly it supports is still open
— treat this as the engine, not the finished product.

## Run it

```bash
go run ./cmd/malten     # start the server on :8080
```

No API key required: with none set it runs a deterministic stub backend, so the
whole thing works offline. Set `ANTHROPIC_API_KEY` (and optionally
`MALTEN_LLM=claude`, `MALTEN_MODEL=...`) to use a real model.

## Build & test

```bash
go build ./...     # build everything
go test ./...      # unit tests + the end-to-end evaluation
go run ./cmd/eval  # print the evaluation report
```

## Layout

```
cmd/malten        server binary (embeds the UI)
cmd/eval          evaluation runner
internal/agent    the bounded agent loop
internal/llm      LLM interface + Stub (deterministic) and Claude backends
internal/tools    Tool interface, Registry, and the capabilities
internal/policy   validation of sensitive actions (the trust boundary)
internal/store    SQLite persistence + schema
internal/server   HTTP handlers + embedded web UI
internal/app      single wiring point (Build) used by server and eval
```

See [ARCHITECTURE.md](ARCHITECTURE.md) for how the pieces fit together.
