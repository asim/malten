-- Malten schema. Everything the single binary needs lives in one SQLite file.

-- Conversation sessions. No login: a session id is minted server-side (or
-- accepted from the client) so history can be tracked without accounts.
CREATE TABLE IF NOT EXISTS sessions (
    id          TEXT PRIMARY KEY,
    customer_id TEXT,
    created_at  DATETIME NOT NULL,
    updated_at  DATETIME NOT NULL
);

-- Full conversation transcript, including tool calls and tool results, stored
-- as JSON-encoded content blocks so multi-turn context can be replayed.
CREATE TABLE IF NOT EXISTS messages (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    role       TEXT NOT NULL,
    content    TEXT NOT NULL,
    created_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id, id);

-- Seed "product" data the agent reads via account_lookup.
CREATE TABLE IF NOT EXISTS accounts (
    customer_id     TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    email           TEXT NOT NULL,
    plan            TEXT NOT NULL,
    status          TEXT NOT NULL,
    seats           INTEGER NOT NULL,
    api_calls_month INTEGER NOT NULL,
    password_reset_at DATETIME
);

CREATE TABLE IF NOT EXISTS orders (
    order_id    TEXT PRIMARY KEY,
    customer_id TEXT NOT NULL,
    description TEXT NOT NULL,
    amount      REAL NOT NULL,
    status      TEXT NOT NULL,
    refunded    INTEGER NOT NULL DEFAULT 0,
    refund_amount REAL NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_orders_customer ON orders(customer_id);

-- Knowledge base chunks searched by kb_search.
CREATE TABLE IF NOT EXISTS kb (
    id      INTEGER PRIMARY KEY AUTOINCREMENT,
    title   TEXT NOT NULL,
    content TEXT NOT NULL
);

-- Support backlog. Tickets and escalations land here for a human to review.
CREATE TABLE IF NOT EXISTS tickets (
    id          TEXT PRIMARY KEY,
    session_id  TEXT,
    customer_id TEXT,
    kind        TEXT NOT NULL,      -- 'ticket' or 'escalation'
    summary     TEXT NOT NULL,
    priority    TEXT NOT NULL,
    status      TEXT NOT NULL,      -- 'open', 'closed'
    created_at  DATETIME NOT NULL
);

-- Immutable audit trail of every tool call, its policy decision and outcome.
CREATE TABLE IF NOT EXISTS audit_log (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id  TEXT,
    customer_id TEXT,
    tool        TEXT NOT NULL,
    input       TEXT NOT NULL,
    decision    TEXT NOT NULL,      -- 'allow', 'deny', 'escalate', 'n/a'
    reason      TEXT,
    result      TEXT,
    is_error    INTEGER NOT NULL DEFAULT 0,
    created_at  DATETIME NOT NULL
);
