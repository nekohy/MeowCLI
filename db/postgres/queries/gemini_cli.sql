-- name: GetGeminiCLI :one
SELECT *
FROM gemini_cli
WHERE id = $1
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
    AND (sqlc.arg(unsynced_only) = false OR synced_at IS NULL);

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
    AND (sqlc.arg(unsynced_only) = false OR synced_at IS NULL)
ORDER BY id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: UpsertGeminiCLI :one
INSERT INTO gemini_cli (id, status, access_token, refresh_token, expired, email, project_id, plan_type, reason, throttled_until, synced_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())
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
UPDATE gemini_cli
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

-- name: DeleteGeminiCLI :execrows
DELETE FROM gemini_cli WHERE id = $1;

-- name: UpdateGeminiCLIStatus :one
UPDATE gemini_cli
SET status = $1, reason = $2
WHERE id = $3
RETURNING *;

-- name: SetGeminiCLIThrottled :exec
UPDATE gemini_cli
SET throttled_until = $1, synced_at = NOW()
WHERE id = $2;

-- name: ListAvailableGeminiCLI :many
SELECT id, email, project_id, plan_type, throttled_until, synced_at
FROM gemini_cli
WHERE status = 'enabled'
  AND (expired > NOW() OR refresh_token != '')
  AND throttled_until <= NOW()
ORDER BY id;
