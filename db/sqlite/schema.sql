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
    reason TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_codex_status_expired ON codex(status, expired);

CREATE TABLE IF NOT EXISTS gemini (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL DEFAULT 'enabled',
    access_token TEXT NOT NULL,
    refresh_token TEXT NOT NULL,
    expired TEXT NOT NULL,
    email TEXT NOT NULL,
    project_id TEXT NOT NULL DEFAULT '',
    plan_type TEXT NOT NULL DEFAULT 'free',
    reason TEXT NOT NULL DEFAULT '',
    synced_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_gemini_status_expired ON gemini(status, expired);

CREATE TABLE IF NOT EXISTS codex_quota (
    credential_id TEXT PRIMARY KEY REFERENCES codex(id) ON DELETE CASCADE,
    quota_5h REAL NOT NULL DEFAULT 1.0,
    quota_7d REAL NOT NULL DEFAULT 1.0,
    quota_spark_5h REAL NOT NULL DEFAULT 1.0,
    quota_spark_7d REAL NOT NULL DEFAULT 1.0,
    reset_5h TEXT,
    reset_7d TEXT,
    reset_spark_5h TEXT,
    reset_spark_7d TEXT,
    throttled_until TEXT NOT NULL DEFAULT (datetime('now')),
    throttled_until_spark TEXT NOT NULL DEFAULT (datetime('now')),
    synced_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_codex_quota_remaining ON codex_quota(quota_5h DESC, quota_7d DESC);

CREATE TABLE IF NOT EXISTS gemini_quota (
    credential_id TEXT PRIMARY KEY REFERENCES gemini(id) ON DELETE CASCADE,
    quota_pro REAL NOT NULL DEFAULT 1.0,
    reset_pro TEXT NOT NULL DEFAULT (datetime('now')),
    quota_flash REAL NOT NULL DEFAULT 1.0,
    reset_flash TEXT NOT NULL DEFAULT (datetime('now')),
    quota_flashlite REAL NOT NULL DEFAULT 1.0,
    reset_flashlite TEXT NOT NULL DEFAULT (datetime('now')),
    throttled_until_pro TEXT NOT NULL DEFAULT (datetime('now')),
    throttled_until_flash TEXT NOT NULL DEFAULT (datetime('now')),
    throttled_until_flashlite TEXT NOT NULL DEFAULT (datetime('now')),
    synced_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_gemini_quota_remaining ON gemini_quota(quota_pro DESC, quota_flash DESC, quota_flashlite DESC);

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
