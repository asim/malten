# Malten — Specification & Scoping

This document records the original scoping for Malten and the task requirements
it was built against. It is the "what and why"; see [ARCHITECTURE.md](ARCHITECTURE.md)
for the "how".

## Scoping (as briefed)

> I am working on a new conversational support agent called Malten. The agent has
> the ability to search within a product knowledge base, lookup account info and
> do things like issue refunds, reset passwords and create tickets. There is also
> situations in which we want to escalate to a human e.g. human in the loop where
> we cannot make the decision without a human authority. The system needs to be
> extensible. Written in Go.
>
> We'll use SQLite as the database. We will use Claude Opus or similar for the
> model, maybe Sonnet if we need to save on cost since it's not essentially a
> high reasoning task yet. We will need a chat interface where questions can be
> posted in new sessions. If we're thinking about this as customer facing, we
> need to be able to track sessions but don't necessarily need an account/login
> system yet. However defining unique session IDs client side and sending that
> through — or a new session ID given by the server side on initial load if one
> is not present — is a good idea so we can track session history in a database.
> If the user is having a problem we'll need to do account lookup via customer id
> so we'd ask for that from the user. We could potentially have email/password
> login and do session storage like that linked to an account but might be too
> much for the task.
>
> Creating tickets would essentially create a backlog in the system which an admin
> or customer support agents could review; users might get emailed a copy of the
> ticket/link. Some of this may need to be stubbed out on first usage as email
> setup will be quite an effort. We want to focus on the LLM loop and UX first.
>
> It will be deployed as a single Go binary on server, so UI baked in.

## How the scoping maps to the build

| Requirement from the brief | Where it lives |
| --- | --- |
| Knowledge-base search | `search` tool (`internal/tools/kb.go`), KB seeded in SQLite |
| Account lookup by customer id | `account_lookup` tool; the agent asks for the id when missing |
| Issue refunds / reset passwords / create tickets | action tools in `internal/tools/actions.go` |
| Human-in-the-loop escalation | `internal/policy` decisions + `escalate_to_human` tool |
| Extensible system | `tools.Registry` + `internal/app` single wiring point |
| Written in Go, SQLite | throughout; pure-Go `modernc.org/sqlite` (no cgo) |
| Claude Opus / Sonnet, not high-reasoning | `internal/llm/claude.go`, thinking off, model configurable |
| Chat interface, new sessions | embedded UI + `POST /api/chat` |
| Session tracking, no login; client- or server-minted session id | `internal/server` session handling; ids in `sessions` table |
| Ticket backlog for admins to review | `tickets` table + `GET /api/tickets` |
| Email a copy of the ticket | **stubbed** (result text returns a link); noted as future work |
| Single Go binary, UI baked in | `//go:embed` UI; one `cmd/malten` binary |

Deliberately deferred (stubbed or out of scope for v1): real email delivery,
email/password login linked to accounts, streaming responses, and a
production-grade KB retriever (the current one is term-overlap search).

## Core requirements (task brief)

The build is a conversational support agent for a SaaS product. A customer sends
a message; the agent resolves the issue, takes an action, or escalates.

The agent has access to:

- **An LLM** (Claude or GPT-4 class). Calls cost money and have latency.
- `search(query, k)` — top-k chunks from the product knowledge base.
- `account_lookup(customer_id)` — subscription, recent orders, usage stats.
- **Action tools** — `issue_refund(order_id, amount)`, `reset_password(customer_id)`,
  `create_ticket(summary, priority)`.
- `escalate_to_human(reason)`.

The LLM and tools can be stubbed. The point is the system around them, not the
model itself.

### The four core requirements and how they are met

1. **Agent loop.** Take a customer message and a `customer_id`, decide which
   tools to call, and produce a final response or an escalation. The loop must
   terminate sensibly.
   → `internal/agent`. Bounded by `MaxSteps` (default 8); exhaustion escalates
   rather than looping. See ARCHITECTURE.md § "The agent loop".

2. **Multi-turn conversation.** The agent remembers context within a session.
   "What about for the second order?" works as a follow-up.
   → Full transcript (including tool calls/results) is persisted per session and
   replayed on each turn. Demonstrated by the "multi-turn follow-up" evaluation
   scenario, where "the second order" resolves against an earlier
   `account_lookup`.

3. **Tool-call validation.** LLM-emitted tool calls are not trusted blindly.
   Destructive actions get validated before execution.
   → `internal/policy` is a hard boundary: every destructive call is checked
   (ownership, amount, approval limit, priority) and returns Allow / Deny /
   Escalate before the tool runs. Unknown tools fail closed.

4. **Evaluation hook.** A way to measure quality, with a defended definition of
   "quality".
   → `internal/eval` + `cmd/eval`. Quality is defined and defended in
   ARCHITECTURE.md § "Evaluation" and in the package doc of `internal/eval`.
