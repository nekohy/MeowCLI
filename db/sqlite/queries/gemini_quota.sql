-- name: UpsertGeminiQuota :one
-- Syncs per-tier quota ratios and reset timestamps for Gemini credentials.
INSERT INTO gemini_quota (credential_id, quota_pro, reset_pro, quota_flash, reset_flash, quota_flashlite, reset_flashlite, synced_at)
VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'))
ON CONFLICT (credential_id) DO UPDATE
SET
    quota_pro = excluded.quota_pro,
    reset_pro = excluded.reset_pro,
    quota_flash = excluded.quota_flash,
    reset_flash = excluded.reset_flash,
    quota_flashlite = excluded.quota_flashlite,
    reset_flashlite = excluded.reset_flashlite,
    synced_at = datetime('now')
RETURNING *;

-- name: SetGeminiQuotaThrottledAll :exec
-- Sets all tier throttles for a Gemini credential.
INSERT INTO gemini_quota (credential_id, throttled_until_pro, throttled_until_flash, throttled_until_flashlite, synced_at)
VALUES (?, ?, ?, ?, datetime('now'))
ON CONFLICT (credential_id) DO UPDATE
SET
    throttled_until_pro = excluded.throttled_until_pro,
    throttled_until_flash = excluded.throttled_until_flash,
    throttled_until_flashlite = excluded.throttled_until_flashlite,
    synced_at = datetime('now');

-- name: SetGeminiQuotaThrottledPro :exec
-- Sets the Pro tier throttle for a Gemini credential.
INSERT INTO gemini_quota (credential_id, throttled_until_pro, synced_at)
VALUES (?, ?, datetime('now'))
ON CONFLICT (credential_id) DO UPDATE
SET
    throttled_until_pro = excluded.throttled_until_pro,
    synced_at = datetime('now');

-- name: SetGeminiQuotaThrottledFlash :exec
-- Sets the Flash tier throttle for a Gemini credential.
INSERT INTO gemini_quota (credential_id, throttled_until_flash, synced_at)
VALUES (?, ?, datetime('now'))
ON CONFLICT (credential_id) DO UPDATE
SET
    throttled_until_flash = excluded.throttled_until_flash,
    synced_at = datetime('now');

-- name: SetGeminiQuotaThrottledFlashLite :exec
-- Sets the Flash Lite tier throttle for a Gemini credential.
INSERT INTO gemini_quota (credential_id, throttled_until_flashlite, synced_at)
VALUES (?, ?, datetime('now'))
ON CONFLICT (credential_id) DO UPDATE
SET
    throttled_until_flashlite = excluded.throttled_until_flashlite,
    synced_at = datetime('now');

-- name: GetGeminiQuota :one
SELECT * FROM gemini_quota WHERE credential_id = ? LIMIT 1;

-- name: ListAvailableGeminiCLI :many
-- Returns enabled, non-throttled Gemini credentials with quota info
SELECT
    g.id,
    g.email,
    g.project_id,
    g.plan_type,
    COALESCE(q.quota_pro, 1.0)                        AS quota_pro,
    COALESCE(q.reset_pro, datetime('now'))             AS reset_pro,
    COALESCE(q.quota_flash, 1.0)                       AS quota_flash,
    COALESCE(q.reset_flash, datetime('now'))           AS reset_flash,
    COALESCE(q.quota_flashlite, 1.0)                   AS quota_flashlite,
    COALESCE(q.reset_flashlite, datetime('now'))       AS reset_flashlite,
    COALESCE(q.throttled_until_pro, datetime('now'))       AS throttled_until_pro,
    COALESCE(q.throttled_until_flash, datetime('now'))     AS throttled_until_flash,
    COALESCE(q.throttled_until_flashlite, datetime('now')) AS throttled_until_flashlite,
    CAST(max(COALESCE(q.throttled_until_pro, datetime('now')), COALESCE(q.throttled_until_flash, datetime('now')), COALESCE(q.throttled_until_flashlite, datetime('now'))) AS TEXT) AS throttled_until,
    COALESCE(q.synced_at, '')                          AS synced_at
FROM gemini g
LEFT JOIN gemini_quota q ON q.credential_id = g.id
WHERE g.status = 'enabled'
  AND (g.expired > datetime('now') OR g.refresh_token != '')
ORDER BY
    COALESCE(q.quota_pro, 1.0) DESC,
    COALESCE(q.quota_flash, 1.0) DESC,
    COALESCE(q.quota_flashlite, 1.0) DESC;

-- name: DeleteGeminiQuota :execrows
-- Deletes the gemini_quota record for a credential.
DELETE FROM gemini_quota WHERE credential_id = ?;
