# Design decisions

A running log of decisions worth remembering — especially ones that bit us or
would bite a future change. Newest first. Keep entries short: context, the
decision, and the consequence to watch for.

---

## The restart-safety rule (read this before adding state)

Malten runs as a single binary that is **restarted on every deploy** (and on
crashes, and when systemd cycles it), while its data lives in a **persistent
SQLite file**. That split is the source of a whole class of bugs:

> **Anything that must be unique, monotonic, or continuous across the life of
> the data must derive from the persisted store or from randomness — never from
> process memory.** Process memory resets to zero on every restart; the database
> does not.

Before adding any in-process counter, cache, sequence, "next id", dedup set, or
rate-limit window, ask: *what happens when the process restarts but the database
keeps its rows?* If the answer is "it collides / double-counts / regresses",
push the state into SQLite or make it random.

What is currently safe, and why:

| State | Where it lives | Restart-safe because |
| --- | --- | --- |
| Session / ticket / escalation ids | `internal/id`, random | 64 bits of `crypto/rand`, so no coordination with the DB is needed |
| Conversation transcript | `messages` table | persisted; replayed per turn |
| Support backlog | `tickets` table | persisted |
| Seed data (accounts/orders/KB) | seeded once, guarded by a row count | re-seed is skipped when data exists |
| Uptime on the status page | process memory | intentionally per-process; it's cosmetic |
| Stub tool-call ids | process memory | in-request only; never persisted or compared across restarts |

---

## ADR-002 — Random ids, not a per-process counter

**Date:** 2026-07 · **Status:** adopted

**Context.** `id.New` originally used an in-process atomic counter
(`SESS-000001`, `SESS-000002`, …). Readable, but the counter resets to zero on
restart while sessions/tickets persist in SQLite. After a deploy, the first new
session minted `SESS-000001` again — a row that already existed — and the insert
failed with `UNIQUE constraint failed: sessions.id`. The user saw the raw SQL
error; a refresh "fixed" it only because the counter had advanced past the
collision. Ticket ids had the same latent bug.

**Decision.** Generate ids from 64 bits of `crypto/rand`, base32-encoded
(`SESS-k7q2m9f3xa4t8`). No coordination with the database, unguessable, and
collision probability is negligible. `server.ensureSession` also retries on the
(astronomically unlikely) duplicate.

**Consequences.** Ids are no longer sortable or sequential — fine, we never
relied on that. This is a specific instance of the restart-safety rule above;
do not reintroduce a persisted-sequence id scheme without storing the sequence
in the database.

---

## ADR-001 — Never leak internal errors to users

**Date:** 2026-07 · **Status:** adopted

**Context.** HTTP handlers returned `err.Error()` straight to the client, so a
database error surfaced verbatim in the chat UI ("constraint failed: …").

**Decision.** Handlers log the real error server-side and return a generic
"something went wrong, please try again" via `server.fail`. Internal details
never reach the browser.

**Consequences.** Debugging relies on server logs (`journalctl -u malten`), so
keep logging the real error. Health/diagnostics live behind `/api/health`
(operational: model, uptime, row counts) and the customer-facing `/status`
page (`/api/status`: operational/degraded only, no internals).

---

## Standing decisions (from the original build)

These are documented in depth in [ARCHITECTURE.md](../ARCHITECTURE.md); listed
here so the record is in one place.

- **The policy layer is a hard boundary.** Every destructive tool call is
  validated (allow/deny/escalate) before it runs; unknown tools fail closed.
  Never execute a destructive tool without a decision.
- **The agent loop is bounded.** `MaxSteps` guarantees termination; exhaustion
  escalates rather than looping.
- **The transcript is the memory.** Multi-turn context is replayed from the
  persisted transcript, not held in process memory (also restart-safe).
- **Single binary, pure-Go SQLite, embedded UI.** No cgo, no external assets.
- **Model is pluggable.** The deterministic stub backend makes the whole system
  runnable and testable offline; Claude is a drop-in via `internal/llm`.
- **No login (yet).** Identity for account actions is an unverified
  `customer_id` the user provides — a deliberate v1 scope, noted as future work.
