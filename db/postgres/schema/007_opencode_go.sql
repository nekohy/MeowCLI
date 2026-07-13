CREATE TABLE IF NOT EXISTS opencode_go (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL DEFAULT 'enabled',
    api_key TEXT NOT NULL,
    auth_cookie TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_opencode_go_status ON opencode_go(status, id);

CREATE TABLE IF NOT EXISTS opencode_go_quota (
    credential_id TEXT PRIMARY KEY REFERENCES opencode_go(id) ON DELETE CASCADE,
    quota_5h FLOAT NOT NULL DEFAULT 1.0,
    quota_7d FLOAT NOT NULL DEFAULT 1.0,
    quota_1mo FLOAT NOT NULL DEFAULT 1.0,
    reset_5h TIMESTAMPTZ,
    reset_7d TIMESTAMPTZ,
    reset_1mo TIMESTAMPTZ,
    rewards_count INTEGER NOT NULL DEFAULT 0,
    throttled_until TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    synced_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_opencode_go_quota_remaining ON opencode_go_quota(quota_5h DESC, quota_7d DESC, quota_1mo DESC);
CREATE INDEX IF NOT EXISTS idx_opencode_go_quota_available_order ON opencode_go_quota(quota_5h DESC, quota_7d DESC, quota_1mo DESC, credential_id);
