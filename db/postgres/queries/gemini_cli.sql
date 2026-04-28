-- name: GetGeminiCLI :one
SELECT *
FROM gemini
WHERE id = $1
LIMIT 1;

-- name: CountEnabledGeminiCLI :one
SELECT COUNT(*)
FROM gemini
WHERE status = 'enabled';

-- name: CountGeminiCLI :one
SELECT COUNT(*) FROM gemini;

-- name: CountGeminiCLIFiltered :one
SELECT COUNT(*)
FROM gemini g
LEFT JOIN gemini_quota q ON q.credential_id = g.id
WHERE
    (sqlc.arg(search) = '' OR LOWER(g.id) LIKE sqlc.arg(search) OR LOWER(g.email) LIKE sqlc.arg(search) OR LOWER(g.status) LIKE sqlc.arg(search) OR LOWER(g.plan_type) LIKE sqlc.arg(search))
    AND (sqlc.arg(status) = '' OR g.status = sqlc.arg(status))
    AND (sqlc.arg(plan_type) = '' OR LOWER(g.plan_type) = LOWER(sqlc.arg(plan_type)))
    AND (sqlc.arg(unsynced_only) = false OR q.synced_at IS NULL OR q.synced_at <= '0001-01-01'::timestamptz);

-- name: ListGeminiCLI :many
SELECT
    g.id, g.status, g.access_token, g.refresh_token, g.expired,
    g.email, g.project_id, g.plan_type, g.reason,
    COALESCE(q.quota_pro, 1.0) AS quota_pro,
    COALESCE(q.reset_pro, NOW()) AS reset_pro,
    COALESCE(q.quota_flash, 1.0) AS quota_flash,
    COALESCE(q.reset_flash, NOW()) AS reset_flash,
    COALESCE(q.quota_flashlite, 1.0) AS quota_flashlite,
    COALESCE(q.reset_flashlite, NOW()) AS reset_flashlite,
    GREATEST(COALESCE(q.throttled_until_pro, '0001-01-01'::timestamptz), COALESCE(q.throttled_until_flash, '0001-01-01'::timestamptz), COALESCE(q.throttled_until_flashlite, '0001-01-01'::timestamptz))::timestamptz AS throttled_until,
    COALESCE(q.synced_at, '0001-01-01'::timestamptz) AS synced_at
FROM gemini g
LEFT JOIN gemini_quota q ON q.credential_id = g.id
ORDER BY g.id;

-- name: ListGeminiCLIPaged :many
SELECT
    g.id, g.status, g.access_token, g.refresh_token, g.expired,
    g.email, g.project_id, g.plan_type, g.reason,
    COALESCE(q.quota_pro, 1.0) AS quota_pro,
    COALESCE(q.reset_pro, NOW()) AS reset_pro,
    COALESCE(q.quota_flash, 1.0) AS quota_flash,
    COALESCE(q.reset_flash, NOW()) AS reset_flash,
    COALESCE(q.quota_flashlite, 1.0) AS quota_flashlite,
    COALESCE(q.reset_flashlite, NOW()) AS reset_flashlite,
    GREATEST(COALESCE(q.throttled_until_pro, '0001-01-01'::timestamptz), COALESCE(q.throttled_until_flash, '0001-01-01'::timestamptz), COALESCE(q.throttled_until_flashlite, '0001-01-01'::timestamptz))::timestamptz AS throttled_until,
    COALESCE(q.synced_at, '0001-01-01'::timestamptz) AS synced_at
FROM gemini g
LEFT JOIN gemini_quota q ON q.credential_id = g.id
WHERE
    (sqlc.arg(search) = '' OR LOWER(g.id) LIKE sqlc.arg(search) OR LOWER(g.email) LIKE sqlc.arg(search) OR LOWER(g.status) LIKE sqlc.arg(search) OR LOWER(g.plan_type) LIKE sqlc.arg(search))
    AND (sqlc.arg(status) = '' OR g.status = sqlc.arg(status))
    AND (sqlc.arg(plan_type) = '' OR LOWER(g.plan_type) = LOWER(sqlc.arg(plan_type)))
    AND (sqlc.arg(unsynced_only) = false OR q.synced_at IS NULL OR q.synced_at <= '0001-01-01'::timestamptz)
ORDER BY g.id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: UpsertGeminiCLI :one
INSERT INTO gemini (id, status, access_token, refresh_token, expired, email, project_id, plan_type, reason, synced_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
ON CONFLICT (id) DO UPDATE
SET
    status = EXCLUDED.status,
    access_token = EXCLUDED.access_token,
    refresh_token = EXCLUDED.refresh_token,
    expired = EXCLUDED.expired,
    email = EXCLUDED.email,
    project_id = EXCLUDED.project_id,
    plan_type = EXCLUDED.plan_type,
    reason = EXCLUDED.reason,
    synced_at = NOW()
RETURNING *;

-- name: UpdateGeminiTokens :one
UPDATE gemini
SET
    status = $1,
    access_token = $2,
    refresh_token = $3,
    expired = $4,
    email = $5,
    project_id = $6,
    plan_type = $7,
    reason = '',
    synced_at = NOW()
WHERE id = $8
RETURNING *;

-- name: UpdateGeminiPlanType :one
UPDATE gemini
SET
    plan_type = $1,
    synced_at = NOW()
WHERE id = $2
RETURNING *;

-- name: DeleteGeminiCLI :execrows
DELETE FROM gemini WHERE id = $1;

-- name: UpdateGeminiCLIStatus :one
UPDATE gemini
SET status = $1, reason = $2
WHERE id = $3
RETURNING *;
