-- name: GetCodex :one
SELECT *
FROM codex
WHERE id = $1
LIMIT 1;

-- name: CountEnabledCodex :one
SELECT COUNT(*)
FROM codex
WHERE status = 'enabled';

-- name: UpdateCodexTokens :one
UPDATE codex
SET
    status = $2,
    access_token = $3,
    expired = $4,
    refresh_token = $5,
    plan_type = $6,
    plan_expired = $7,
    reason = ''
WHERE id = $1
RETURNING *;

-- name: CountCodex :one
SELECT COUNT(*) FROM codex;

-- name: CountCodexFiltered :one
SELECT COUNT(*)
FROM codex c
LEFT JOIN quota q ON q.credential_id = c.id
WHERE
    (sqlc.arg(search) = '' OR LOWER(c.id) LIKE sqlc.arg(search) OR LOWER(c.status) LIKE sqlc.arg(search) OR LOWER(c.plan_type) LIKE sqlc.arg(search))
    AND (sqlc.arg(status) = '' OR c.status = sqlc.arg(status))
    AND (sqlc.arg(plan_type) = '' OR LOWER(c.plan_type) = LOWER(sqlc.arg(plan_type)))
    AND ((NOT sqlc.arg(unsynced_only)::boolean) OR q.synced_at IS NULL OR q.synced_at <= '0001-01-01'::timestamptz);

-- name: ListCodex :many
SELECT
    c.*,
    COALESCE(q.quota_5h, 1.0) AS quota_5h,
    COALESCE(q.quota_7d, 1.0) AS quota_7d,
    COALESCE(q.reset_5h, '0001-01-01'::timestamptz) AS reset_5h,
    COALESCE(q.reset_7d, '0001-01-01'::timestamptz) AS reset_7d,
    COALESCE(q.throttled_until, '0001-01-01'::timestamptz) AS throttled_until,
    COALESCE(q.synced_at, '0001-01-01'::timestamptz) AS synced_at
FROM codex c
LEFT JOIN quota q ON q.credential_id = c.id
ORDER BY c.id;

-- name: ListCodexPaged :many
SELECT
    c.*,
    COALESCE(q.quota_5h, 1.0) AS quota_5h,
    COALESCE(q.quota_7d, 1.0) AS quota_7d,
    COALESCE(q.reset_5h, '0001-01-01'::timestamptz) AS reset_5h,
    COALESCE(q.reset_7d, '0001-01-01'::timestamptz) AS reset_7d,
    COALESCE(q.throttled_until, '0001-01-01'::timestamptz) AS throttled_until,
    COALESCE(q.synced_at, '0001-01-01'::timestamptz) AS synced_at
FROM codex c
LEFT JOIN quota q ON q.credential_id = c.id
WHERE
    (sqlc.arg(search) = '' OR LOWER(c.id) LIKE sqlc.arg(search) OR LOWER(c.status) LIKE sqlc.arg(search) OR LOWER(c.plan_type) LIKE sqlc.arg(search))
    AND (sqlc.arg(status) = '' OR c.status = sqlc.arg(status))
    AND (sqlc.arg(plan_type) = '' OR LOWER(c.plan_type) = LOWER(sqlc.arg(plan_type)))
    AND ((NOT sqlc.arg(unsynced_only)::boolean) OR q.synced_at IS NULL OR q.synced_at <= '0001-01-01'::timestamptz)
ORDER BY c.id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CreateCodex :one
INSERT INTO codex (id, status, access_token, expired, refresh_token, plan_type, plan_expired)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: DeleteCodex :execrows
DELETE FROM codex WHERE id = $1;

-- name: UpdateCodexStatus :one
UPDATE codex SET status = $2, reason = $3 WHERE id = $1
RETURNING *;
