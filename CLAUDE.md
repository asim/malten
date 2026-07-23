# CLAUDE.md

Guidance for Claude Code (and other AI agents) working in this repository.

## What this is

Malten is a conversational customer-support agent for a SaaS product, written in
Go and shipping as a single binary with an embedded chat UI and a SQLite
database. A customer sends a message; the agent resolves it, takes an action, or
escalates to a human.

Start with [SPEC.md](SPEC.md) (scope), then [ARCHITECTURE.md](ARCHITECTURE.md)
(how it fits together), then [README.md](README.md) (run/extend).

## Repository map

```
cmd/malten        server binary (embeds the UI)
cmd/eval          evaluation runner
internal/agent    the bounded agent loop
internal/llm      LLM interface + Stub (deterministic) and Claude backends
internal/tools    Tool interface, Registry, and the six capabilities
internal/policy   validation of destructive actions (the trust boundary)
internal/store    SQLite persistence + seed data + schema.sql
internal/server   HTTP handlers + web/index.html (embedded)
internal/app      single wiring point (Build) used by server and eval
internal/eval     evaluation harness, scenarios, metrics
internal/id       short process-unique ids
```

## Build, test, run

```bash
go build ./...            # build everything
go test ./...             # unit tests + full end-to-end eval (stub backend)
go run ./cmd/eval         # print the evaluation report
go run ./cmd/malten       # start the server on :8080 (stub backend by default)
```

No API key is required: with none set, the deterministic stub backend runs and
the app is fully usable offline. Set `ANTHROPIC_API_KEY` (and optionally
`MALTEN_LLM=claude`, `MALTEN_MODEL=...`) to use the real model.

`go test ./...` must stay green — the eval suite in `internal/eval` is the
end-to-end contract and asserts **zero safety violations**.

## Conventions & invariants (do not break these)

- **The policy layer is a hard boundary.** Destructive tool calls
  (`issue_refund`, `reset_password`, `create_ticket`) must go through
  `policy.Validate` before executing. Never execute a destructive tool without a
  decision. Unknown destructive tools fail closed (Deny).
- **The agent loop must terminate.** Keep the `MaxSteps` bound; on exhaustion the
  agent escalates rather than looping.
- **The transcript is the memory.** Persist assistant tool-call turns and their
  results as content blocks so multi-turn context replays. Don't drop tool blocks
  from history.
- **The LLM interface stays narrow and provider-agnostic.** The Stub and Claude
  backends must both satisfy `llm.LLM`; anything the agent needs from the model
  goes through `Complete`.
- **Extensibility is the point.** A new capability is a `Tool` implementation, a
  `Register` call in `internal/app`, and (if destructive) a `policy.Validate`
  case. Prefer that shape over special-casing in the agent.
- **Single binary.** Keep SQLite pure-Go (`modernc.org/sqlite`, no cgo) and keep
  the UI embedded (`//go:embed`). Don't add cgo or external asset dependencies.
- **Restart safety.** The binary restarts on every deploy while the SQLite data
  persists. Anything that must be unique, monotonic, or continuous across the
  data's lifetime must derive from the store or from randomness — never a
  process-memory counter (that reset-on-restart is exactly what caused the id
  collision). See [docs/DECISIONS.md](docs/DECISIONS.md).

## Model usage

- Default model is `claude-opus-4-8`; Sonnet is acceptable for cost. Thinking is
  intentionally **off** (support is not high-reasoning) — keep it off unless
  there's a measured reason.
- The Claude backend lives only in `internal/llm/claude.go`. Keep model-specific
  code there.

## When adding features

1. Implement the `Tool` (schema + `Destructive()` + `Execute`).
2. Register it in `internal/app/app.go`.
3. If destructive, add validation in `internal/policy`.
4. Add an evaluation scenario in `internal/eval/scenarios.go` covering the happy
   path and any deny/escalate path.
5. Update README.md / ARCHITECTURE.md if behaviour or surface changed.
6. `go test ./...` and `go vet ./...` before finishing.

## Known stubs / future work

- Email delivery (ticket copies, reset links) is stubbed — results return a link
  string only.
- No account/login; identity for actions is a `customer_id` the user provides.
- KB retrieval is term-overlap, not embeddings.
