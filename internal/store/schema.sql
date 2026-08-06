-- Malten schema. Everything the single binary needs lives in one SQLite file.

-- Conversation sessions. No login: a session id is minted server-side (or
-- accepted from the client) so history can be tracked without accounts.
CREATE TABLE IF NOT EXISTS sessions (
    id         TEXT PRIMARY KEY,
    title      TEXT,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
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

-- A small library of self-help resources the agent can search to ground its
-- suggestions (grounding, breathing, planning, sleep, reaching out).
CREATE TABLE IF NOT EXISTS kb (
    id      INTEGER PRIMARY KEY AUTOINCREMENT,
    title   TEXT NOT NULL,
    content TEXT NOT NULL
);

-- Issues: the things you're working through. The agent logs them, optionally
-- with a plan, so you can come back to them.
CREATE TABLE IF NOT EXISTS issues (
    id         TEXT PRIMARY KEY,
    session_id TEXT,
    title      TEXT NOT NULL,
    plan       TEXT,
    status     TEXT NOT NULL,      -- 'open' or 'closed'
    created_at DATETIME NOT NULL
);

-- Immutable audit trail of every tool call and its outcome.
CREATE TABLE IF NOT EXISTS audit_log (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT,
    tool       TEXT NOT NULL,
    input      TEXT NOT NULL,
    decision   TEXT NOT NULL,      -- 'allow', 'deny', 'n/a'
    reason     TEXT,
    result     TEXT,
    is_error   INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL
);
