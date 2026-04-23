CREATE TABLE IF NOT EXISTS models (
    alias TEXT PRIMARY KEY,
    origin TEXT NOT NULL,
    handler TEXT NOT NULL,
    plan_types TEXT NOT NULL DEFAULT '',
    extra TEXT NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_models_handler ON models(handler);

CREATE TABLE IF NOT EXISTS codex (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL DEFAULT 'enabled',
    access_token TEXT NOT NULL,
    expired TEXT NOT NULL,
    refresh_token TEXT NOT NULL,
    plan_type TEXT NOT NULL,
    plan_expired TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_codex_status_expired ON codex(status, expired);

CREATE TABLE IF NOT EXISTS gemini_cli (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL DEFAULT 'enabled',
    access_token TEXT NOT NULL,
    refresh_token TEXT NOT NULL,
    expired TEXT NOT NULL,
    email TEXT NOT NULL,
    project_id TEXT NOT NULL DEFAULT '',
    plan_type TEXT NOT NULL DEFAULT 'free',
    reason TEXT NOT NULL DEFAULT '',
    throttled_until TEXT NOT NULL DEFAULT (datetime('now')),
    synced_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_gemini_cli_status_expired ON gemini_cli(status, expired);

CREATE TABLE IF NOT EXISTS quota (
    credential_id TEXT PRIMARY KEY,
    quota_5h REAL NOT NULL DEFAULT 1.0,
    quota_7d REAL NOT NULL DEFAULT 1.0,
    reset_5h TEXT NOT NULL DEFAULT (datetime('now')),
    reset_7d TEXT NOT NULL DEFAULT (datetime('now')),
    throttled_until TEXT NOT NULL DEFAULT (datetime('now')),
    synced_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_quota_remaining ON quota(quota_5h DESC, quota_7d DESC);

CREATE TABLE IF NOT EXISTS auth_keys (
    key  TEXT PRIMARY KEY,
    role TEXT NOT NULL DEFAULT 'user',
    note TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
