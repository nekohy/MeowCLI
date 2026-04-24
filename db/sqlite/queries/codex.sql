-- name: GetCodex :one
SELECT *
FROM codex
WHERE id = ?
LIMIT 1;

-- name: CountEnabledCodex :one
SELECT COUNT(*)
FROM codex
WHERE status = 'enabled';

-- name: UpdateCodexTokens :one
UPDATE codex
SET
    status = ?,
    access_token = ?,
    expired = ?,
    refresh_token = ?,
    plan_type = ?,
    reason = ''
WHERE id = ?
RETURNING *;

-- name: CountCodex :one
SELECT COUNT(*) FROM codex;

-- name: CountCodexFiltered :one
SELECT COUNT(*)
FROM codex c
LEFT JOIN codex_quota q ON q.credential_id = c.id
WHERE
    (sqlc.arg(search) = '' OR LOWER(c.id) LIKE sqlc.arg(search) OR LOWER(c.status) LIKE sqlc.arg(search) OR LOWER(c.plan_type) LIKE sqlc.arg(search))
    AND (sqlc.arg(status) = '' OR c.status = sqlc.arg(status))
    AND (sqlc.arg(plan_type) = '' OR LOWER(c.plan_type) = LOWER(sqlc.arg(plan_type)))
    AND (sqlc.arg(unsynced_only) = 0 OR q.synced_at IS NULL OR q.synced_at = '');

-- name: ListCodex :many
SELECT
    c.*,
    COALESCE(q.quota_5h, 1.0) AS quota_5h,
    COALESCE(q.quota_7d, 1.0) AS quota_7d,
    COALESCE(q.quota_spark_5h, 1.0) AS quota_spark_5h,
    COALESCE(q.quota_spark_7d, 1.0) AS quota_spark_7d,
    COALESCE(q.reset_5h, '') AS reset_5h,
    COALESCE(q.reset_7d, '') AS reset_7d,
    COALESCE(q.reset_spark_5h, '') AS reset_spark_5h,
    COALESCE(q.reset_spark_7d, '') AS reset_spark_7d,
    COALESCE(q.throttled_until, '') AS throttled_until,
    COALESCE(q.synced_at, '') AS synced_at
FROM codex c
LEFT JOIN codex_quota q ON q.credential_id = c.id
ORDER BY c.id;

-- name: ListCodexPaged :many
SELECT
    c.*,
    COALESCE(q.quota_5h, 1.0) AS quota_5h,
    COALESCE(q.quota_7d, 1.0) AS quota_7d,
    COALESCE(q.quota_spark_5h, 1.0) AS quota_spark_5h,
    COALESCE(q.quota_spark_7d, 1.0) AS quota_spark_7d,
    COALESCE(q.reset_5h, '') AS reset_5h,
    COALESCE(q.reset_7d, '') AS reset_7d,
    COALESCE(q.reset_spark_5h, '') AS reset_spark_5h,
    COALESCE(q.reset_spark_7d, '') AS reset_spark_7d,
    COALESCE(q.throttled_until, '') AS throttled_until,
    COALESCE(q.synced_at, '') AS synced_at
FROM codex c
LEFT JOIN codex_quota q ON q.credential_id = c.id
WHERE
    (sqlc.arg(search) = '' OR LOWER(c.id) LIKE sqlc.arg(search) OR LOWER(c.status) LIKE sqlc.arg(search) OR LOWER(c.plan_type) LIKE sqlc.arg(search))
    AND (sqlc.arg(status) = '' OR c.status = sqlc.arg(status))
    AND (sqlc.arg(plan_type) = '' OR LOWER(c.plan_type) = LOWER(sqlc.arg(plan_type)))
    AND (sqlc.arg(unsynced_only) = 0 OR q.synced_at IS NULL OR q.synced_at = '')
ORDER BY c.id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CreateCodex :one
INSERT INTO codex (id, status, access_token, expired, refresh_token, plan_type)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: DeleteCodex :execrows
DELETE FROM codex WHERE id = ?;

-- name: UpdateCodexStatus :one
UPDATE codex SET status = ?, reason = ? WHERE id = ?
RETURNING *;
