-- name: GetGeminiCLI :one
SELECT *
FROM gemini
WHERE id = ?
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
    AND (sqlc.arg(unsynced_only) = 0 OR q.synced_at IS NULL OR q.synced_at = '');

-- name: ListGeminiCLI :many
SELECT
    g.id, g.status, g.access_token, g.refresh_token, g.expired,
    g.email, g.project_id, g.plan_type, g.reason,
    COALESCE(q.quota_pro, 1.0) AS quota_pro,
    COALESCE(q.reset_pro, datetime('now')) AS reset_pro,
    COALESCE(q.quota_flash, 1.0) AS quota_flash,
    COALESCE(q.reset_flash, datetime('now')) AS reset_flash,
    COALESCE(q.quota_flashlite, 1.0) AS quota_flashlite,
    COALESCE(q.reset_flashlite, datetime('now')) AS reset_flashlite,
    CAST(max(COALESCE(q.throttled_until_pro, ''), COALESCE(q.throttled_until_flash, ''), COALESCE(q.throttled_until_flashlite, '')) AS TEXT) AS throttled_until,
    COALESCE(q.synced_at, '') AS synced_at
FROM gemini g
LEFT JOIN gemini_quota q ON q.credential_id = g.id
ORDER BY g.id;

-- name: ListGeminiCLIPaged :many
SELECT
    g.id, g.status, g.access_token, g.refresh_token, g.expired,
    g.email, g.project_id, g.plan_type, g.reason,
    COALESCE(q.quota_pro, 1.0) AS quota_pro,
    COALESCE(q.reset_pro, datetime('now')) AS reset_pro,
    COALESCE(q.quota_flash, 1.0) AS quota_flash,
    COALESCE(q.reset_flash, datetime('now')) AS reset_flash,
    COALESCE(q.quota_flashlite, 1.0) AS quota_flashlite,
    COALESCE(q.reset_flashlite, datetime('now')) AS reset_flashlite,
    CAST(max(COALESCE(q.throttled_until_pro, ''), COALESCE(q.throttled_until_flash, ''), COALESCE(q.throttled_until_flashlite, '')) AS TEXT) AS throttled_until,
    COALESCE(q.synced_at, '') AS synced_at
FROM gemini g
LEFT JOIN gemini_quota q ON q.credential_id = g.id
WHERE
    (sqlc.arg(search) = '' OR LOWER(g.id) LIKE sqlc.arg(search) OR LOWER(g.email) LIKE sqlc.arg(search) OR LOWER(g.status) LIKE sqlc.arg(search) OR LOWER(g.plan_type) LIKE sqlc.arg(search))
    AND (sqlc.arg(status) = '' OR g.status = sqlc.arg(status))
    AND (sqlc.arg(plan_type) = '' OR LOWER(g.plan_type) = LOWER(sqlc.arg(plan_type)))
    AND (sqlc.arg(unsynced_only) = 0 OR q.synced_at IS NULL OR q.synced_at = '')
ORDER BY g.id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: UpsertGeminiCLI :one
INSERT INTO gemini (id, status, access_token, refresh_token, expired, email, project_id, plan_type, reason, synced_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
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
    synced_at = datetime('now')
RETURNING *;

-- name: UpdateGeminiTokens :one
UPDATE gemini
SET
    status = ?,
    access_token = ?,
    refresh_token = ?,
    expired = ?,
    email = ?,
    project_id = ?,
    plan_type = ?,
    reason = '',
    synced_at = datetime('now')
WHERE id = ?
RETURNING *;

-- name: UpdateGeminiPlanType :one
UPDATE gemini
SET
    plan_type = ?,
    synced_at = datetime('now')
WHERE id = ?
RETURNING *;

-- name: DeleteGeminiCLI :execrows
DELETE FROM gemini WHERE id = ?;

-- name: UpdateGeminiCLIStatus :one
UPDATE gemini
SET status = ?, reason = ?
WHERE id = ?
RETURNING *;

-- name: RestoreExpiredThrottledGeminiCLI :exec
UPDATE gemini
SET status = 'enabled', reason = ''
WHERE status = 'throttled'
  AND id IN (
    SELECT g.id
    FROM gemini g
    LEFT JOIN gemini_quota q ON q.credential_id = g.id
    WHERE g.status = 'throttled'
      AND COALESCE(q.throttled_until_pro, datetime('now')) <= datetime('now')
      AND COALESCE(q.throttled_until_flash, datetime('now')) <= datetime('now')
      AND COALESCE(q.throttled_until_flashlite, datetime('now')) <= datetime('now')
  );
