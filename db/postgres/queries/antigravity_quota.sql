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
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW())
ON CONFLICT (credential_id) DO UPDATE
SET
    quota_claude = EXCLUDED.quota_claude,
    reset_claude = EXCLUDED.reset_claude,
    quota_pro = EXCLUDED.quota_pro,
    reset_pro = EXCLUDED.reset_pro,
    quota_flash = EXCLUDED.quota_flash,
    reset_flash = EXCLUDED.reset_flash,
    quota_flashlite = EXCLUDED.quota_flashlite,
    reset_flashlite = EXCLUDED.reset_flashlite,
    quota_tab = EXCLUDED.quota_tab,
    reset_tab = EXCLUDED.reset_tab,
    quota_image = EXCLUDED.quota_image,
    reset_image = EXCLUDED.reset_image,
    synced_at = NOW()
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
VALUES (
    sqlc.arg(credential_id),
    sqlc.arg(throttled_until_claude),
    sqlc.arg(throttled_until_pro),
    sqlc.arg(throttled_until_flash),
    sqlc.arg(throttled_until_flashlite),
    sqlc.arg(throttled_until_tab),
    sqlc.arg(throttled_until_image),
    NOW()
)
ON CONFLICT (credential_id) DO UPDATE
SET
    throttled_until_claude = EXCLUDED.throttled_until_claude,
    throttled_until_pro = EXCLUDED.throttled_until_pro,
    throttled_until_flash = EXCLUDED.throttled_until_flash,
    throttled_until_flashlite = EXCLUDED.throttled_until_flashlite,
    throttled_until_tab = EXCLUDED.throttled_until_tab,
    throttled_until_image = EXCLUDED.throttled_until_image,
    synced_at = NOW();

-- name: SetAntigravityQuotaThrottledClaude :exec
INSERT INTO antigravity_quota (credential_id, throttled_until_claude, synced_at)
VALUES ($1, $2, NOW())
ON CONFLICT (credential_id) DO UPDATE
SET
    throttled_until_claude = EXCLUDED.throttled_until_claude,
    synced_at = NOW();

-- name: SetAntigravityQuotaThrottledPro :exec
INSERT INTO antigravity_quota (credential_id, throttled_until_pro, synced_at)
VALUES ($1, $2, NOW())
ON CONFLICT (credential_id) DO UPDATE
SET
    throttled_until_pro = EXCLUDED.throttled_until_pro,
    synced_at = NOW();

-- name: SetAntigravityQuotaThrottledFlash :exec
INSERT INTO antigravity_quota (credential_id, throttled_until_flash, synced_at)
VALUES ($1, $2, NOW())
ON CONFLICT (credential_id) DO UPDATE
SET
    throttled_until_flash = EXCLUDED.throttled_until_flash,
    synced_at = NOW();

-- name: SetAntigravityQuotaThrottledFlashLite :exec
INSERT INTO antigravity_quota (credential_id, throttled_until_flashlite, synced_at)
VALUES ($1, $2, NOW())
ON CONFLICT (credential_id) DO UPDATE
SET
    throttled_until_flashlite = EXCLUDED.throttled_until_flashlite,
    synced_at = NOW();

-- name: SetAntigravityQuotaThrottledTab :exec
INSERT INTO antigravity_quota (credential_id, throttled_until_tab, synced_at)
VALUES ($1, $2, NOW())
ON CONFLICT (credential_id) DO UPDATE
SET
    throttled_until_tab = EXCLUDED.throttled_until_tab,
    synced_at = NOW();

-- name: SetAntigravityQuotaThrottledImage :exec
INSERT INTO antigravity_quota (credential_id, throttled_until_image, synced_at)
VALUES ($1, $2, NOW())
ON CONFLICT (credential_id) DO UPDATE
SET
    throttled_until_image = EXCLUDED.throttled_until_image,
    synced_at = NOW();

-- name: ClearAntigravityQuotaThrottle :exec
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
VALUES ($1, NOW(), NOW(), NOW(), NOW(), NOW(), NOW(), NOW())
ON CONFLICT (credential_id) DO UPDATE
SET
    throttled_until_claude = NOW(),
    throttled_until_pro = NOW(),
    throttled_until_flash = NOW(),
    throttled_until_flashlite = NOW(),
    throttled_until_tab = NOW(),
    throttled_until_image = NOW(),
    synced_at = NOW();

-- name: ListAvailableAntigravity :many
SELECT
    a.id,
    a.email,
    a.project_id,
    a.plan_type,
    COALESCE(q.quota_claude, 1.0) AS quota_claude,
    COALESCE(q.reset_claude, NOW()) AS reset_claude,
    COALESCE(q.quota_pro, 1.0) AS quota_pro,
    COALESCE(q.reset_pro, NOW()) AS reset_pro,
    COALESCE(q.quota_flash, 1.0) AS quota_flash,
    COALESCE(q.reset_flash, NOW()) AS reset_flash,
    COALESCE(q.quota_flashlite, 1.0) AS quota_flashlite,
    COALESCE(q.reset_flashlite, NOW()) AS reset_flashlite,
    COALESCE(q.quota_tab, 1.0) AS quota_tab,
    COALESCE(q.reset_tab, NOW() + INTERVAL '100 years') AS reset_tab,
    COALESCE(q.quota_image, 1.0) AS quota_image,
    COALESCE(q.reset_image, NOW()) AS reset_image,
    COALESCE(c.credits_amount, 0) AS credits_amount,
    COALESCE(c.credit_types, '') AS credit_types,
    COALESCE(q.throttled_until_claude, NOW()) AS throttled_until_claude,
    COALESCE(q.throttled_until_pro, NOW()) AS throttled_until_pro,
    COALESCE(q.throttled_until_flash, NOW()) AS throttled_until_flash,
    COALESCE(q.throttled_until_flashlite, NOW()) AS throttled_until_flashlite,
    COALESCE(q.throttled_until_tab, NOW()) AS throttled_until_tab,
    COALESCE(q.throttled_until_image, NOW()) AS throttled_until_image,
    GREATEST(
        COALESCE(q.throttled_until_claude, NOW()),
        COALESCE(q.throttled_until_pro, NOW()),
        COALESCE(q.throttled_until_flash, NOW()),
        COALESCE(q.throttled_until_flashlite, NOW()),
        COALESCE(q.throttled_until_tab, NOW()),
        COALESCE(q.throttled_until_image, NOW())
    )::timestamptz AS throttled_until,
    COALESCE(q.synced_at, '0001-01-01'::timestamptz) AS synced_at
FROM antigravity a
LEFT JOIN antigravity_quota q ON q.credential_id = a.id
LEFT JOIN antigravity_credits c ON c.credential_id = a.id
WHERE a.status = 'enabled'
  AND (a.expired > NOW() OR a.refresh_token != '')
ORDER BY
    COALESCE(q.quota_claude, 1.0) DESC,
    COALESCE(q.quota_pro, 1.0) DESC,
    COALESCE(q.quota_flash, 1.0) DESC,
    COALESCE(q.quota_flashlite, 1.0) DESC,
    COALESCE(q.quota_tab, 1.0) DESC,
    COALESCE(q.quota_image, 1.0) DESC;

-- name: DeleteAntigravityQuota :execrows
DELETE FROM antigravity_quota WHERE credential_id = $1;
