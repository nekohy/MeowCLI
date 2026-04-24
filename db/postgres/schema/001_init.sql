CREATE TABLE IF NOT EXISTS models (
    alias TEXT PRIMARY KEY,
    origin TEXT NOT NULL,
    handler TEXT NOT NULL,
    plan_types TEXT NOT NULL DEFAULT '',
    extra JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_models_handler ON models(handler);

CREATE TABLE IF NOT EXISTS codex (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL DEFAULT 'enabled',
    access_token TEXT NOT NULL,
    expired TIMESTAMPTZ NOT NULL,
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
    expired TIMESTAMPTZ NOT NULL,
    email TEXT NOT NULL,
    project_id TEXT NOT NULL DEFAULT '',
    plan_type TEXT NOT NULL DEFAULT 'free',
    reason TEXT NOT NULL DEFAULT '',
    synced_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_gemini_status_expired ON gemini(status, expired);

CREATE TABLE IF NOT EXISTS codex_quota (
    credential_id TEXT PRIMARY KEY REFERENCES codex(id) ON DELETE CASCADE,
    quota_5h  FLOAT NOT NULL DEFAULT 1.0,
    quota_7d  FLOAT NOT NULL DEFAULT 1.0,
    quota_spark_5h FLOAT NOT NULL DEFAULT 1.0,
    quota_spark_7d FLOAT NOT NULL DEFAULT 1.0,
    reset_5h TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reset_7d TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reset_spark_5h TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reset_spark_7d TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    throttled_until TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    throttled_until_spark TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    synced_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_codex_quota_remaining ON codex_quota(quota_5h DESC, quota_7d DESC);

CREATE TABLE IF NOT EXISTS auth_keys (
    key  TEXT PRIMARY KEY,
    role TEXT NOT NULL DEFAULT 'user',
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
