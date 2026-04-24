-- name: UpsertGeminiQuota :one
-- Syncs per-tier quota ratios and reset timestamps for Gemini credentials.
INSERT INTO gemini_quota (credential_id, quota_pro, reset_pro, quota_flash, reset_flash, quota_flashlite, reset_flashlite, synced_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
ON CONFLICT (credential_id) DO UPDATE
SET
    quota_pro = EXCLUDED.quota_pro,
    reset_pro = EXCLUDED.reset_pro,
    quota_flash = EXCLUDED.quota_flash,
    reset_flash = EXCLUDED.reset_flash,
    quota_flashlite = EXCLUDED.quota_flashlite,
    reset_flashlite = EXCLUDED.reset_flashlite,
    synced_at = NOW()
RETURNING *;

-- name: SetGeminiQuotaThrottled :exec
-- Sets throttled_until for a Gemini credential in the quota table.
INSERT INTO gemini_quota (credential_id, throttled_until, synced_at)
VALUES ($1, $2, NOW())
ON CONFLICT (credential_id) DO UPDATE
SET
    throttled_until = EXCLUDED.throttled_until,
    synced_at = NOW();

-- name: GetGeminiQuota :one
SELECT * FROM gemini_quota WHERE credential_id = $1 LIMIT 1;

-- name: ListAvailableGeminiCLI :many
-- Returns enabled, non-throttled Gemini credentials with quota info
SELECT
    g.id,
    g.email,
    g.project_id,
    g.plan_type,
    COALESCE(q.quota_pro, 1.0)           AS quota_pro,
    COALESCE(q.reset_pro, NOW())         AS reset_pro,
    COALESCE(q.quota_flash, 1.0)         AS quota_flash,
    COALESCE(q.reset_flash, NOW())       AS reset_flash,
    COALESCE(q.quota_flashlite, 1.0)     AS quota_flashlite,
    COALESCE(q.reset_flashlite, NOW())   AS reset_flashlite,
    COALESCE(q.throttled_until, NOW())   AS throttled_until,
    COALESCE(q.synced_at, '0001-01-01'::timestamptz) AS synced_at
FROM gemini g
LEFT JOIN gemini_quota q ON q.credential_id = g.id
WHERE g.status = 'enabled'
  AND (g.expired > NOW() OR g.refresh_token != '')
  AND COALESCE(q.throttled_until, NOW()) <= NOW()
ORDER BY
    COALESCE(q.quota_pro, 1.0) DESC,
    COALESCE(q.quota_flash, 1.0) DESC,
    COALESCE(q.quota_flashlite, 1.0) DESC;

-- name: DeleteGeminiQuota :execrows
-- Deletes the gemini_quota record for a credential.
DELETE FROM gemini_quota WHERE credential_id = $1;
