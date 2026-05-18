CREATE TABLE IF NOT EXISTS antigravity (
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

CREATE INDEX IF NOT EXISTS idx_antigravity_status_expired ON antigravity(status, expired);
CREATE INDEX IF NOT EXISTS idx_antigravity_available_cover ON antigravity(status, expired, id, email, project_id, plan_type);

CREATE TABLE IF NOT EXISTS antigravity_quota (
    credential_id TEXT PRIMARY KEY REFERENCES antigravity(id) ON DELETE CASCADE,
    quota_claude FLOAT NOT NULL DEFAULT 1.0,
    reset_claude TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    quota_pro FLOAT NOT NULL DEFAULT 1.0,
    reset_pro TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    quota_flash FLOAT NOT NULL DEFAULT 1.0,
    reset_flash TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    quota_flashlite FLOAT NOT NULL DEFAULT 1.0,
    reset_flashlite TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    quota_tab FLOAT NOT NULL DEFAULT 1.0,
    reset_tab TIMESTAMPTZ NOT NULL DEFAULT (NOW() + INTERVAL '100 years'),
    quota_image FLOAT NOT NULL DEFAULT 1.0,
    reset_image TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    throttled_until_claude TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    throttled_until_pro TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    throttled_until_flash TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    throttled_until_flashlite TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    throttled_until_tab TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    throttled_until_image TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    synced_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_antigravity_quota_remaining ON antigravity_quota(quota_claude DESC, quota_pro DESC, quota_flash DESC, quota_flashlite DESC, quota_tab DESC, quota_image DESC);
CREATE INDEX IF NOT EXISTS idx_antigravity_quota_available_order ON antigravity_quota(quota_claude DESC, quota_pro DESC, quota_flash DESC, quota_flashlite DESC, quota_tab DESC, quota_image DESC, credential_id);

CREATE TABLE IF NOT EXISTS antigravity_credits (
    credential_id TEXT PRIMARY KEY REFERENCES antigravity(id) ON DELETE CASCADE,
    credits_amount FLOAT NOT NULL DEFAULT 0,
    credit_types TEXT NOT NULL DEFAULT '',
    synced_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_antigravity_credits_amount ON antigravity_credits(credits_amount DESC, credential_id);
