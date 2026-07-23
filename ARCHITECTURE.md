# Malten — Architecture

Malten is a conversational customer-support agent for a SaaS product. A customer
sends a message; the agent resolves the issue, takes an action, or escalates to
a human. This document explains how it is put together and why.

For scope and requirements see [SPEC.md](SPEC.md); for how to run and extend it
see [README.md](README.md).

## Design goals

- **The system, not the model, is the product.** The LLM is pluggable and can be
  a deterministic stub, so the loop, validation, persistence and UX can be built
  and tested independently of any model.
- **Never trust the model with destructive actions.** Every account-changing
  tool call passes a policy check first. Human-in-the-loop is a first-class
  outcome, not an afterthought.
- **Extensible.** New capabilities are a `Tool` implementation plus one
  registration line. New policy rules live in one place.
- **One binary.** Pure-Go SQLite (`modernc.org/sqlite`, no cgo) and an embedded
  UI mean `go build` produces a single deployable artifact.

## Component map

```
                 ┌──────────────┐    HTTP + embedded chat UI
   customer ───▶ │   server     │    POST /api/chat, /api/tickets, /api/session
                 └──────┬───────┘
                        ▼
                 ┌──────────────┐    the loop: decide → validate → act → repeat
                 │    agent     │    bounded by MaxSteps; escalates on exhaustion
                 └──┬───────┬───┘
        model turn  │       │  destructive call?
             ▼      │       ▼
        ┌────────┐  │  ┌──────────┐   Allow / Deny / Escalate
        │  llm   │  │  │  policy  │   (ownership, amount, limits, priority)
        │ stub / │  │  └────┬─────┘
        │ claude │  │       │ allow
        └────────┘  ▼       ▼
                 ┌──────────────┐    kb_search, account_lookup, issue_refund,
                 │    tools     │    reset_password, create_ticket, escalate
                 │  (registry)  │
                 └──────┬───────┘
                        ▼
                 ┌──────────────┐    sessions, transcript, accounts, orders,
                 │    store     │    kb, tickets (backlog), audit_log
                 │  (SQLite)    │
                 └──────────────┘
```

Package layout (`internal/`):

| Package | Responsibility |
| --- | --- |
| `llm` | Provider-agnostic model interface; `Stub` (deterministic) and `Claude` backends. |
| `tools` | `Tool` interface + `Registry`; the six capabilities. |
| `policy` | Validation of destructive tool calls — the trust boundary. |
| `store` | SQLite persistence; sessions, transcript, seed data, backlog, audit. |
| `agent` | The orchestration loop. |
| `server` | HTTP handlers + embedded chat UI. |
| `app` | Single wiring point used by the server and the eval harness. |
| `eval` | Evaluation harness, scenarios, metrics. |
| `id` | Short process-unique ids. |

Binaries: `cmd/malten` (server), `cmd/eval` (evaluation runner).

## The agent loop

`agent.Handle(ctx, sessionID, customerID, message)` runs one user message to a
terminal state:

1. Load the persisted transcript for the session and append the user message.
2. Repeat up to `MaxSteps` (default 8) times:
   - Ask the model for the next turn, given the system prompt (with customer
     context), the transcript, and the tool definitions.
   - **No tool calls** → this is the final answer. Persist it and return.
   - **Tool calls** → for each one:
     - `escalate_to_human` → record an escalation and return (terminal).
     - Unknown tool → return an error tool-result; the model can adapt.
     - Destructive tool → ask `policy.Validate`:
       - **Deny** → return an error tool-result explaining why; loop continues.
       - **Escalate** → record an escalation and return (terminal).
       - **Allow** → execute.
     - Read-only tool → execute directly.
   - Feed the tool results back as the next user turn and continue.
3. If the step budget is exhausted, escalate rather than loop forever.

**Termination** is guaranteed by the step bound; the worst case is a graceful
escalation. Every tool execution and policy decision is written to `audit_log`.

The **transcript is the memory.** Because assistant tool-call turns and their
results are persisted as content blocks and replayed on the next message,
multi-turn context works without a separate memory store: "refund the second
order" resolves against an `account_lookup` from an earlier turn.

## The LLM interface

`llm.LLM` is intentionally tiny: `Complete(ctx, Request) (*Response, error)`,
where a `Request` is the system prompt, the message history, and the tool
definitions, and a `Response` is either final text or a set of tool calls. This
mirrors the Anthropic Messages API content model closely enough that the Claude
backend is a thin adapter, while staying small enough that the `Stub` can
satisfy it deterministically.

- **`Stub`** (`internal/llm/stub.go`) — a rule-based backend. It reads the latest
  customer intent, inspects which tools have already run this turn, and emits the
  next sensible tool call or a final message. It is not a good model; it is a
  stand-in that makes the whole system runnable and the evaluation reproducible
  with no API key and no network.
- **`Claude`** (`internal/llm/claude.go`) — the real backend via the Anthropic Go
  SDK. Thinking is left **off**: support is not a high-reasoning task, so we
  favour latency and cost. The model id is configurable (default
  `claude-opus-4-8`; set `MALTEN_MODEL` to a Sonnet id to save cost).

Swapping backends changes nothing else in the system — the agent, tools, policy
and eval are identical.

## Tools & extensibility

A `Tool` declares its model-facing definition (name, description, JSON-Schema
input), whether it is **destructive** (must be validated), and how to execute.
The six capabilities:

| Tool | Destructive | Notes |
| --- | --- | --- |
| `kb_search` | no | term-overlap search over the KB, top-k |
| `account_lookup` | no | plan, usage, orders as JSON |
| `issue_refund` | yes | validated: ownership, amount, approval limit |
| `reset_password` | yes | validated: account exists, same customer |
| `create_ticket` | yes | validated: priority; writes to backlog |
| `escalate_to_human` | — | terminal; handled by the agent directly |

Adding a capability: implement `Tool`, register it in `internal/app`, and (if
destructive) add a case to `policy.Validate`. Nothing else needs to change.

Per-call context (which session/customer) is passed via `context.Context`
(`tools.CallInfo`) so the `Tool` interface stays stateless and narrow.

## Policy: the trust boundary

`policy.Validate(ctx, customerID, tool, input)` returns **Allow**, **Deny**, or
**Escalate**. This is where model output stops being trusted:

- **`issue_refund`** — the order must exist, belong to *this* customer, and not
  already be refunded; the amount must be positive and not exceed the order
  total; a refund above the auto-approval limit (default **$200**) is
  **Escalated**, not executed.
- **`reset_password`** — the account must exist; the model may not target a
  different customer than the session's.
- **`create_ticket`** — priority must be one of low/normal/high/urgent.
- Unknown destructive tools **fail closed** (Deny).

This is the mechanism behind "human-in-the-loop": some actions are simply not
the agent's to authorize, and the policy says so by escalating. An escalation
lands in the same `tickets` backlog a human reviews.

## Persistence (SQLite)

One database file holds everything (schema in `internal/store/schema.sql`):

- `sessions`, `messages` — session tracking and the full transcript (content
  blocks stored as JSON so tool calls/results replay).
- `accounts`, `orders`, `kb` — seeded "product" data the read-only tools serve.
- `tickets` — the support backlog; both support tickets and escalations.
- `audit_log` — an append-only record of every tool call, its policy decision,
  and its result.

Connections are capped at one to keep SQLite writes simple under concurrent HTTP
requests. Use `:memory:` for tests and the eval harness.

## Sessions (no login)

Per the brief there is no account/login system. A session id is either supplied
by the client or minted server-side on first contact and returned (also set as a
convenience cookie). History is keyed by that id. Customer identity for account
actions comes from a `customer_id` the user provides in-conversation — the agent
asks for it when a request needs it and none is known.

## HTTP surface

| Method & path | Purpose |
| --- | --- |
| `GET /` | the embedded chat UI |
| `POST /api/chat` | `{session_id?, customer_id?, message}` → agent `Reply` |
| `GET /api/session/{id}` | full transcript for a session |
| `GET /tickets`, `GET /api/tickets` | the backlog (tickets + escalations), page and JSON |
| `GET /status`, `GET /api/status` | customer-facing status (operational/degraded) |
| `GET /api/health` | operational check: model, uptime, row counts |

## Evaluation

The evaluation hook (`internal/eval`, run via `cmd/eval` or `go test`) measures
the **system's** behaviour, which is what determines whether a support agent is
safe to ship. Each scenario is a scripted conversation with expectations, run
against a fresh in-memory database.

**Quality is defined as, in priority order:**

1. **Safety (highest).** An agent that can move money and change accounts must
   never take a destructive action it isn't allowed to. Any forbidden-action
   execution is a hard failure — worse than any number of unhelpful answers.
2. **Escalation accuracy.** Escalate exactly when a human is required (over-limit
   refund, explicit request, unresolvable) and not otherwise. Both false
   negatives (acting without authority) and false positives (dumping resolvable
   work on humans) are failures.
3. **Task resolution / tool selection.** For issues it can handle, did it pick
   the right tool and reach a sensible terminal state?
4. **Efficiency.** Fewer model turns for the same outcome is better — reported as
   a secondary metric, not a pass/fail gate.

**Why this ordering:** for a customer-facing agent with real side effects, the
expensive errors are unauthorized/incorrect destructive actions and failures to
escalate. Helpfulness matters, but a helpful agent that occasionally issues a
refund it shouldn't is not shippable; a cautious agent that escalates a hard
case is. So safety and escalation gate the release; resolution and efficiency
tune it.

The same harness can be pointed at the real Claude backend
(`MALTEN_LLM=claude go run ./cmd/eval`) to measure the model against the
identical bar. The release gate (`Report.OK()`) is: every scenario passes and
zero safety violations.

## Design decisions & restart safety

A running log of decisions worth remembering — especially ones that bit us or
would bite a future change. Read this before adding state.

### The restart-safety rule

Malten runs as a single binary that is **restarted on every deploy** (and on
crashes, and when systemd cycles it), while its data lives in a **persistent
SQLite file**. That split is the source of a whole class of bugs:

> **Anything that must be unique, monotonic, or continuous across the life of
> the data must derive from the persisted store or from randomness — never from
> process memory.** Process memory resets to zero on every restart; the database
> does not.

Before adding any in-process counter, cache, "next id", sequence, dedup set, or
rate-limit window, ask: *what happens when the process restarts but the database
keeps its rows?* If the answer is "it collides / double-counts / regresses",
push the state into SQLite or make it random.

What is currently safe, and why:

| State | Where it lives | Restart-safe because |
| --- | --- | --- |
| Session / ticket / escalation ids | `internal/id`, random | 64 bits of `crypto/rand`, no coordination with the DB needed |
| Conversation transcript | `messages` table | persisted; replayed per turn |
| Support backlog | `tickets` table | persisted |
| Seed data (accounts/orders/KB) | seeded once, guarded by a row count | re-seed is skipped when data exists |
| Uptime on the status page | process memory | intentionally per-process; cosmetic |
| Stub tool-call ids | process memory | in-request only; never persisted or compared across restarts |

### ADR-002 — Random ids, not a per-process counter

`id.New` originally used an in-process atomic counter (`SESS-000001`, …).
Readable, but the counter resets to zero on restart while sessions/tickets
persist in SQLite. After a deploy, the first new session minted `SESS-000001`
again — a row that already existed — and the insert failed with `UNIQUE
constraint failed: sessions.id`; the user saw the raw SQL error, and a refresh
"fixed" it only because the counter had advanced past the collision. Ticket ids
had the same latent bug.

**Decision:** generate ids from 64 bits of `crypto/rand`, base32-encoded
(`SESS-k7q2m9f3xa4t8`) — no DB coordination, unguessable, negligible collision
probability. `server.ensureSession` also retries on a duplicate. Ids are no
longer sequential, which we never relied on. Do not reintroduce a
persisted-sequence id scheme without storing the sequence in the database.

### ADR-001 — Never leak internal errors to users

Handlers originally returned `err.Error()` straight to the client, so a database
error surfaced verbatim in the chat UI. **Decision:** handlers log the real error
server-side and return a generic "something went wrong, please try again" via
`server.fail`. Debugging relies on server logs (`journalctl -u malten`), so keep
logging the real error. Operational detail lives behind `/api/health` (model,
uptime, row counts) and the customer-facing `/status` page (`/api/status`:
operational/degraded only, no internals).
