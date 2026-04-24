CREATE TABLE IF NOT EXISTS gemini_quota (
    credential_id TEXT PRIMARY KEY REFERENCES gemini(id) ON DELETE CASCADE,
    quota_pro  FLOAT NOT NULL DEFAULT 1.0,
    reset_pro  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    quota_flash FLOAT NOT NULL DEFAULT 1.0,
    reset_flash TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    quota_flashlite FLOAT NOT NULL DEFAULT 1.0,
    reset_flashlite TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    throttled_until TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    synced_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_gemini_quota_remaining ON gemini_quota(quota_pro DESC, quota_flash DESC, quota_flashlite DESC);
