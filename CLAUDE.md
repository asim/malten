# CLAUDE.md

Guidance for AI agents working in this repository. Kept in-repo because it
carries engineering invariants, not product docs.

## What this is

Malten is a conversational support companion for **neurodivergent people**
(ADHD, autism and related), written in Go and shipping as a single binary with
an embedded chat UI and a SQLite database. Someone sends a message; the agent
helps them name what they're stuck on, think it through, and shape a small next
step — logging things worth revisiting as "issues". It is not a therapist,
diagnostician or crisis service, and must never present itself as one; crisis
messages are met with care and a signpost to real help.

The moat is the **context around the model** (neurodivergent-aware framing, the
knowledge base, the issues/plans structure, session memory) — not the raw LLM.

## Repository map

```
cmd/malten        server binary (embeds the UI)
cmd/eval          evaluation runner
internal/agent    the bounded agent loop + system prompt
internal/llm      LLM interface + Stub (deterministic) and Claude backends
internal/tools    Tool interface, Registry, and the capabilities (search, create_issue)
internal/policy   validation boundary for destructive actions (fail-closed)
internal/store    SQLite persistence + schema.sql + self-help seed
internal/server   HTTP handlers + embedded web UI
internal/app      single wiring point (Build) used by server and eval
internal/eval     evaluation harness, scenarios, metrics
internal/id       short random ids
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

- **Safety first.** The agent must never diagnose, must not act as a crisis
  service, and must respond to crisis/self-harm signals with care and a signpost
  to emergency help. The system prompt and the crisis eval scenario enforce this.
- **The policy layer is a hard boundary.** Any tool marked `Destructive()` must
  pass `policy.Validate` before executing; unknown destructive tools fail closed
  (Deny). There are no destructive tools today — keep the seam anyway.
- **The agent loop must terminate.** Keep the `MaxSteps` bound; on exhaustion it
  closes the turn gently rather than looping.
- **The transcript is the memory.** Persist assistant tool-call turns and their
  results as content blocks so multi-turn context replays. Every `tool_use` must
  be answered by a `tool_result` in the next message, or the Anthropic API 400s.
- **The LLM interface stays narrow and provider-agnostic.** Stub and Claude both
  satisfy `llm.LLM`; anything the agent needs goes through `Complete`.
- **Extensibility is the point.** A new capability is a `Tool` implementation, a
  `Register` call in `internal/app`, and (if destructive) a `policy.Validate`
  case. Prefer that over special-casing the agent.
- **Single binary.** Keep SQLite pure-Go (`modernc.org/sqlite`, no cgo) and the
  UI embedded (`//go:embed`). No cgo, no external asset/CDN dependencies.
- **Restart safety.** The binary restarts on deploy while the SQLite data
  persists. Anything unique/monotonic across the data's lifetime must derive
  from the store or randomness (`internal/id`), never a process-memory counter.

## Model usage

- Default model is `claude-opus-4-8`; Sonnet is acceptable for cost. Thinking is
  intentionally **off** — keep it off unless there's a measured reason.
- The Claude backend lives only in `internal/llm/claude.go`.

## When adding a capability

1. Implement the `Tool` (schema + `Destructive()` + `Execute`).
2. Register it in `internal/app/app.go`.
3. If destructive, add a `policy.Validate` case.
4. Add an eval scenario in `internal/eval/scenarios.go`.
5. `go test ./...` and `go vet ./...` before finishing.

## Known limitations / future work

- KB retrieval is term-overlap, not embeddings.
- No streaming responses or conversation sidebar yet (planned).
- No accounts or subscriptions yet (subscriptions planned).
- License is AGPL-3.0.
