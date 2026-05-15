CREATE TABLE IF NOT EXISTS inbox_items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'open',
    kind TEXT NOT NULL DEFAULT 'task',
    priority TEXT NOT NULL DEFAULT 'normal',
    source TEXT NOT NULL DEFAULT 'agent',
    repo TEXT NOT NULL DEFAULT '',
    session_id TEXT NOT NULL DEFAULT '',
    tags TEXT NOT NULL DEFAULT '[]',
    linked_log_ids TEXT NOT NULL DEFAULT '[]',
    linked_fingerprints TEXT NOT NULL DEFAULT '[]',
    created_by TEXT NOT NULL DEFAULT '',
    claimed_by TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    due_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_inbox_items_status ON inbox_items(status);
CREATE INDEX IF NOT EXISTS idx_inbox_items_kind ON inbox_items(kind);
CREATE INDEX IF NOT EXISTS idx_inbox_items_repo ON inbox_items(repo);
CREATE INDEX IF NOT EXISTS idx_inbox_items_session_id ON inbox_items(session_id);
CREATE INDEX IF NOT EXISTS idx_inbox_items_updated_at ON inbox_items(updated_at DESC);
