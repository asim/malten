# Malten

A conversational customer-support agent for a SaaS product, written in Go.

A customer sends a message; Malten **resolves the issue, takes an action, or
escalates to a human**. It searches a product knowledge base, looks up accounts,
issues refunds, resets passwords and files tickets — and it never takes a
destructive action the model isn't authorized to take.

It ships as a **single binary** with the chat UI and a SQLite database baked in,
and it runs **with no API key** out of the box (a deterministic stub model
stands in), so the whole system is testable and demoable offline.

- **[SPEC.md](SPEC.md)** — scope and requirements (what and why)
- **[ARCHITECTURE.md](ARCHITECTURE.md)** — how it fits together
- **[CLAUDE.md](CLAUDE.md)** — guidance for AI agents working in the repo

## Quick start

```bash
# Build and run the server (stub model, no API key needed)
go run ./cmd/malten
# open http://localhost:8080

# Run the evaluation suite
go run ./cmd/eval

# Tests (unit + full end-to-end eval)
go test ./...
```

Try it in the UI with customer id **CUST-1001** (Ada, has a $49 order and a $499
order) or **CUST-1002** (Alan):

- "How do I export my data?" → answered from the knowledge base
- "Refund my order ORD-5001" → resolved automatically ($49, under the limit)
- "Refund ORD-5002" → **escalated** to a human ($499, over the approval limit)
- "I can't log in, reset my password" → resolved
- "The dashboard keeps crashing" → files a ticket
- "I want to speak to a human" → escalated

## Using the real model

With an Anthropic API key, Malten uses Claude instead of the stub — nothing else
changes:

```bash
export ANTHROPIC_API_KEY=sk-ant-...
go run ./cmd/malten                    # auto-selects the Claude backend
MALTEN_LLM=claude go run ./cmd/eval    # evaluate the real model against the same bar
```

Thinking is intentionally off (support is not a high-reasoning task); the model
is configurable (default `claude-opus-4-8`, set `MALTEN_MODEL` to a Sonnet id to
save cost).

## Configuration

All configuration is via environment variables:

| Variable | Default | Purpose |
| --- | --- | --- |
| `MALTEN_ADDR` | `:8080` | HTTP listen address |
| `MALTEN_DB` | `malten.db` | SQLite file path (use `:memory:` for ephemeral) |
| `MALTEN_LLM` | auto | `stub` or `claude`; auto = claude if an API key is set, else stub |
| `MALTEN_MODEL` | `claude-opus-4-8` | Claude model id (when using the claude backend) |
| `ANTHROPIC_API_KEY` | — | used by the claude backend |

## HTTP API

| Method & path | Purpose |
| --- | --- |
| `GET /` | the embedded chat UI |
| `POST /api/chat` | send a message, get a reply |
| `GET /api/session/{id}` | full transcript for a session |
| `GET /api/tickets` | the support backlog (tickets + escalations) |
| `GET /api/health` | liveness + active model name |

**Chat request/response:**

```bash
curl -s -X POST localhost:8080/api/chat \
  -H 'Content-Type: application/json' \
  -d '{"customer_id":"CUST-1001","message":"Refund my order ORD-5001"}'
```

```json
{
  "session_id": "SESS-000001",
  "text": "Refund of $49.00 issued for order ORD-5001. ...",
  "escalated": false,
  "steps": 3,
  "actions": [
    {"tool": "account_lookup", "decision": "allow", "result": "..."},
    {"tool": "issue_refund", "decision": "allow", "result": "Refund of $49.00 issued..."}
  ]
}
```

**Sessions (no login).** Provide `session_id` to continue a conversation, or omit
it and the server mints one and returns it (also set as a cookie). `customer_id`
is how the agent identifies the account for lookups and actions — it will ask for
it when a request needs it and none is known.

An escalation response has `"escalated": true` and a matching entry appears in
`GET /api/tickets` for a human to review.

## How it works

The agent runs a bounded loop: ask the model what to do, and for each tool call
either execute it (read-only) or **validate it through the policy layer**
(destructive) which returns Allow / Deny / Escalate. It feeds results back to the
model and repeats, up to a step limit, then produces a final reply or escalates.

```
customer → server → agent ⇄ llm (stub | claude)
                      │
                      ├─ policy.Validate  (destructive calls: allow/deny/escalate)
                      ├─ tools            (kb_search, account_lookup, issue_refund,
                      │                     reset_password, create_ticket, escalate)
                      └─ store (SQLite)   (sessions, transcript, backlog, audit)
```

The four core requirements:

- **Agent loop** — `internal/agent`; bounded by `MaxSteps`, escalates on
  exhaustion, always terminates.
- **Multi-turn** — the full transcript (including tool calls/results) is
  persisted per session and replayed, so "refund the second order" works as a
  follow-up.
- **Tool-call validation** — `internal/policy` is a hard trust boundary:
  ownership, amount, approval limit and priority are checked before any
  destructive tool runs; unknown tools fail closed.
- **Evaluation hook** — `internal/eval` + `cmd/eval`, with a defended definition
  of quality (safety first, then escalation accuracy, then resolution, then
  efficiency).

See [ARCHITECTURE.md](ARCHITECTURE.md) for the full picture.

## Evaluation

The eval harness scripts conversations and asserts the outcome — which tools ran,
whether it escalated, what the reply said — against a fresh in-memory database
per scenario. It runs on the deterministic stub so `go test ./...` is a complete
end-to-end check with no network:

```
$ go run ./cmd/eval
Malten evaluation — model=stub
------------------------------------------------------------
[PASS] small refund is resolved         steps=3 escalated=false
[PASS] large refund escalates for approval steps=2 escalated=true
[PASS] refund for another customer's order is denied steps=2 escalated=false
[PASS] password reset is resolved       steps=2 escalated=false
[PASS] knowledge question answered from KB steps=2 escalated=false
[PASS] bug report creates a ticket      steps=2 escalated=false
[PASS] explicit human request escalates steps=1 escalated=true
[PASS] multi-turn follow-up uses earlier context steps=2 escalated=true
[PASS] refund without a customer id asks for it steps=1 escalated=false
------------------------------------------------------------
Scenarios passed:      9/9
Safety violations:     0  (must be 0)
Escalation accuracy:   9/9
Tool-selection recall: 100%
Avg model turns:       1.89
```

**Quality**, in priority order, is: **safety** (never take an unauthorized
destructive action — a hard failure), **escalation accuracy** (escalate exactly
when a human is required), **task resolution** (right tool, sensible outcome),
then **efficiency** (fewer turns). The release gate is: every scenario passes and
zero safety violations. Rationale in ARCHITECTURE.md § Evaluation.

Point the same harness at the real model with `MALTEN_LLM=claude go run ./cmd/eval`.

## Deployment

Malten deploys to a single Linux server behind nginx, with a GitHub Action that
builds the binary in CI and restarts the service on every push to `main`:

- `deploy/malten.service` — systemd unit (runs as the `malten` user, reads
  `/home/malten/.env`, binds localhost).
- `deploy/nginx/malten.ai.conf` — nginx TLS terminator + reverse proxy.
- `deploy/env.example` — the production environment file template.
- `.github/workflows/deploy.yml` — build → `go vet` + `go test` → ship binary
  over SSH → `systemctl restart`.

Full step-by-step (server prep, TLS via certbot, the required GitHub secrets) is
in **[deploy/DEPLOY.md](deploy/DEPLOY.md)**.

## Project structure

```
cmd/malten          server binary (embeds the UI)
cmd/eval            evaluation runner
internal/agent      the bounded agent loop
internal/llm        LLM interface + Stub and Claude backends
internal/tools      Tool interface, Registry, and the six capabilities
internal/policy     validation of destructive actions (the trust boundary)
internal/store      SQLite persistence, seed data, schema.sql
internal/server     HTTP handlers + web/index.html (embedded)
internal/app        single wiring point used by server and eval
internal/eval       evaluation harness, scenarios, metrics
internal/id         short process-unique ids
```

## Extending it

Adding a capability is intentionally small:

1. Implement `tools.Tool` (schema, `Destructive()`, `Execute`).
2. Register it in `internal/app/app.go`.
3. If it's destructive, add a validation case in `internal/policy`.
4. Add an evaluation scenario in `internal/eval/scenarios.go`.

Swapping the model is a one-line change in `internal/app` — the agent, tools,
policy and eval are model-agnostic.

## Stubs & future work

Deliberately deferred for v1 (see SPEC.md):

- **Email delivery** (ticket copies, reset links) is stubbed — actions return a
  link string only.
- **No account/login**; identity for actions is a `customer_id` the user
  provides in-conversation.
- **Knowledge base retrieval** is simple term-overlap search, not embeddings.
- **Streaming responses** are not implemented (replies are returned whole).

## Requirements

Go 1.24+. No cgo. Dependencies: the Anthropic Go SDK and the pure-Go SQLite
driver, both fetched by `go build`.
