-- name: GetGeminiCLI :one
SELECT *
FROM gemini_cli
WHERE id = ?
LIMIT 1;

-- name: CountEnabledGeminiCLI :one
SELECT COUNT(*)
FROM gemini_cli
WHERE status = 'enabled';

-- name: CountGeminiCLI :one
SELECT COUNT(*) FROM gemini_cli;

-- name: CountGeminiCLIFiltered :one
SELECT COUNT(*)
FROM gemini_cli
WHERE
    (sqlc.arg(search) = '' OR LOWER(id) LIKE sqlc.arg(search) OR LOWER(email) LIKE sqlc.arg(search) OR LOWER(status) LIKE sqlc.arg(search) OR LOWER(plan_type) LIKE sqlc.arg(search))
    AND (sqlc.arg(status) = '' OR status = sqlc.arg(status))
    AND (sqlc.arg(plan_type) = '' OR LOWER(plan_type) = LOWER(sqlc.arg(plan_type)))
    AND (sqlc.arg(unsynced_only) = 0 OR synced_at IS NULL OR synced_at = '');

-- name: ListGeminiCLI :many
SELECT *
FROM gemini_cli
ORDER BY id;

-- name: ListGeminiCLIPaged :many
SELECT *
FROM gemini_cli
WHERE
    (sqlc.arg(search) = '' OR LOWER(id) LIKE sqlc.arg(search) OR LOWER(email) LIKE sqlc.arg(search) OR LOWER(status) LIKE sqlc.arg(search) OR LOWER(plan_type) LIKE sqlc.arg(search))
    AND (sqlc.arg(status) = '' OR status = sqlc.arg(status))
    AND (sqlc.arg(plan_type) = '' OR LOWER(plan_type) = LOWER(sqlc.arg(plan_type)))
    AND (sqlc.arg(unsynced_only) = 0 OR synced_at IS NULL OR synced_at = '')
ORDER BY id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: UpsertGeminiCLI :one
INSERT INTO gemini_cli (id, status, access_token, refresh_token, expired, email, project_id, plan_type, reason, throttled_until, synced_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))
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
UPDATE gemini_cli
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

-- name: DeleteGeminiCLI :execrows
DELETE FROM gemini_cli WHERE id = ?;

-- name: UpdateGeminiCLIStatus :one
UPDATE gemini_cli
SET status = ?, reason = ?
WHERE id = ?
RETURNING *;

-- name: SetGeminiCLIThrottled :exec
UPDATE gemini_cli
SET throttled_until = ?, synced_at = datetime('now')
WHERE id = ?;

-- name: ListAvailableGeminiCLI :many
SELECT id, email, project_id, plan_type, throttled_until, synced_at
FROM gemini_cli
WHERE status = 'enabled'
  AND (expired > datetime('now') OR refresh_token != '')
  AND throttled_until <= datetime('now')
ORDER BY id;
