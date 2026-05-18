-- name: UpsertAntigravityQuota :one
INSERT INTO antigravity_quota (
    credential_id,
    quota_claude, reset_claude,
    quota_pro, reset_pro,
    quota_flash, reset_flash,
    quota_flashlite, reset_flashlite,
    quota_tab, reset_tab,
    quota_image, reset_image,
    synced_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
ON CONFLICT (credential_id) DO UPDATE
SET
    quota_claude = excluded.quota_claude,
    reset_claude = excluded.reset_claude,
    quota_pro = excluded.quota_pro,
    reset_pro = excluded.reset_pro,
    quota_flash = excluded.quota_flash,
    reset_flash = excluded.reset_flash,
    quota_flashlite = excluded.quota_flashlite,
    reset_flashlite = excluded.reset_flashlite,
    quota_tab = excluded.quota_tab,
    reset_tab = excluded.reset_tab,
    quota_image = excluded.quota_image,
    reset_image = excluded.reset_image,
    synced_at = datetime('now')
RETURNING *;

-- name: SetAntigravityQuotaThrottledAll :exec
INSERT INTO antigravity_quota (
    credential_id,
    throttled_until_claude,
    throttled_until_pro,
    throttled_until_flash,
    throttled_until_flashlite,
    throttled_until_tab,
    throttled_until_image,
    synced_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'))
ON CONFLICT (credential_id) DO UPDATE
SET
    throttled_until_claude = excluded.throttled_until_claude,
    throttled_until_pro = excluded.throttled_until_pro,
    throttled_until_flash = excluded.throttled_until_flash,
    throttled_until_flashlite = excluded.throttled_until_flashlite,
    throttled_until_tab = excluded.throttled_until_tab,
    throttled_until_image = excluded.throttled_until_image,
    synced_at = datetime('now');

-- name: SetAntigravityQuotaThrottledClaude :exec
INSERT INTO antigravity_quota (credential_id, throttled_until_claude, synced_at)
VALUES (?, ?, datetime('now'))
ON CONFLICT (credential_id) DO UPDATE
SET
    throttled_until_claude = excluded.throttled_until_claude,
    synced_at = datetime('now');

-- name: SetAntigravityQuotaThrottledPro :exec
INSERT INTO antigravity_quota (credential_id, throttled_until_pro, synced_at)
VALUES (?, ?, datetime('now'))
ON CONFLICT (credential_id) DO UPDATE
SET
    throttled_until_pro = excluded.throttled_until_pro,
    synced_at = datetime('now');

-- name: SetAntigravityQuotaThrottledFlash :exec
INSERT INTO antigravity_quota (credential_id, throttled_until_flash, synced_at)
VALUES (?, ?, datetime('now'))
ON CONFLICT (credential_id) DO UPDATE
SET
    throttled_until_flash = excluded.throttled_until_flash,
    synced_at = datetime('now');

-- name: SetAntigravityQuotaThrottledFlashLite :exec
INSERT INTO antigravity_quota (credential_id, throttled_until_flashlite, synced_at)
VALUES (?, ?, datetime('now'))
ON CONFLICT (credential_id) DO UPDATE
SET
    throttled_until_flashlite = excluded.throttled_until_flashlite,
    synced_at = datetime('now');

-- name: SetAntigravityQuotaThrottledTab :exec
INSERT INTO antigravity_quota (credential_id, throttled_until_tab, synced_at)
VALUES (?, ?, datetime('now'))
ON CONFLICT (credential_id) DO UPDATE
SET
    throttled_until_tab = excluded.throttled_until_tab,
    synced_at = datetime('now');

-- name: SetAntigravityQuotaThrottledImage :exec
INSERT INTO antigravity_quota (credential_id, throttled_until_image, synced_at)
VALUES (?, ?, datetime('now'))
ON CONFLICT (credential_id) DO UPDATE
SET
    throttled_until_image = excluded.throttled_until_image,
    synced_at = datetime('now');

-- name: ListAvailableAntigravity :many
SELECT
    a.id,
    a.email,
    a.project_id,
    a.plan_type,
    COALESCE(q.quota_claude, 1.0) AS quota_claude,
    COALESCE(q.reset_claude, datetime('now')) AS reset_claude,
    COALESCE(q.quota_pro, 1.0) AS quota_pro,
    COALESCE(q.reset_pro, datetime('now')) AS reset_pro,
    COALESCE(q.quota_flash, 1.0) AS quota_flash,
    COALESCE(q.reset_flash, datetime('now')) AS reset_flash,
    COALESCE(q.quota_flashlite, 1.0) AS quota_flashlite,
    COALESCE(q.reset_flashlite, datetime('now')) AS reset_flashlite,
    COALESCE(q.quota_tab, 1.0) AS quota_tab,
    COALESCE(q.reset_tab, datetime('now', '+100 years')) AS reset_tab,
    COALESCE(q.quota_image, 1.0) AS quota_image,
    COALESCE(q.reset_image, datetime('now')) AS reset_image,
    COALESCE(c.credits_amount, 0) AS credits_amount,
    COALESCE(c.credit_types, '') AS credit_types,
    COALESCE(q.throttled_until_claude, datetime('now')) AS throttled_until_claude,
    COALESCE(q.throttled_until_pro, datetime('now')) AS throttled_until_pro,
    COALESCE(q.throttled_until_flash, datetime('now')) AS throttled_until_flash,
    COALESCE(q.throttled_until_flashlite, datetime('now')) AS throttled_until_flashlite,
    COALESCE(q.throttled_until_tab, datetime('now')) AS throttled_until_tab,
    COALESCE(q.throttled_until_image, datetime('now')) AS throttled_until_image,
    CAST(max(
        COALESCE(q.throttled_until_claude, datetime('now')),
        COALESCE(q.throttled_until_pro, datetime('now')),
        COALESCE(q.throttled_until_flash, datetime('now')),
        COALESCE(q.throttled_until_flashlite, datetime('now')),
        COALESCE(q.throttled_until_tab, datetime('now')),
        COALESCE(q.throttled_until_image, datetime('now'))
    ) AS TEXT) AS throttled_until,
    COALESCE(q.synced_at, '') AS synced_at
FROM antigravity a
LEFT JOIN antigravity_quota q ON q.credential_id = a.id
LEFT JOIN antigravity_credits c ON c.credential_id = a.id
WHERE a.status = 'enabled'
  AND (a.expired > datetime('now') OR a.refresh_token != '')
ORDER BY
    COALESCE(q.quota_claude, 1.0) DESC,
    COALESCE(q.quota_pro, 1.0) DESC,
    COALESCE(q.quota_flash, 1.0) DESC,
    COALESCE(q.quota_flashlite, 1.0) DESC,
    COALESCE(q.quota_tab, 1.0) DESC,
    COALESCE(q.quota_image, 1.0) DESC;

-- name: DeleteAntigravityQuota :execrows
DELETE FROM antigravity_quota WHERE credential_id = ?;
