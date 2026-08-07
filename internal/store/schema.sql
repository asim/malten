-- Malten schema. The server persists nothing about users: the only table is the
-- read-only self-help library, seeded at startup into an in-memory database.
-- Conversations, issues and memory all live on the client, not here.
CREATE TABLE IF NOT EXISTS kb (
    id      INTEGER PRIMARY KEY AUTOINCREMENT,
    title   TEXT NOT NULL,
    content TEXT NOT NULL
);
