-- name: GetOpenCodeGo :one
SELECT *
FROM opencode_go
WHERE id = ?
LIMIT 1;

-- name: CountEnabledOpenCodeGo :one
SELECT COUNT(*)
FROM opencode_go
WHERE status = 'enabled';

-- name: CountOpenCodeGo :one
SELECT COUNT(*) FROM opencode_go;

-- name: CountOpenCodeGoFiltered :one
SELECT COUNT(*)
FROM opencode_go o
LEFT JOIN opencode_go_quota q ON q.credential_id = o.id
WHERE
    (sqlc.arg(search) = '' OR LOWER(o.id) LIKE sqlc.arg(search) OR LOWER(o.status) LIKE sqlc.arg(search))
    AND (
        sqlc.arg(statuses) = ''
        OR (
            (instr(',' || sqlc.arg(statuses) || ',', ',enabled,') = 0 OR o.status = 'enabled')
            AND (instr(',' || sqlc.arg(statuses) || ',', ',disabled,') = 0 OR o.status = 'disabled')
            AND (instr(',' || sqlc.arg(statuses) || ',', ',throttled:all,') = 0 OR q.throttled_until > datetime('now'))
            AND (instr(',' || sqlc.arg(statuses) || ',', ',throttled:default,') = 0 OR q.throttled_until > datetime('now'))
        )
    )
    AND (sqlc.arg(unsynced_only) = 0 OR q.synced_at IS NULL OR q.synced_at = '');

-- name: ListOpenCodeGo :many
SELECT
    o.id, o.status, o.api_key, o.auth_cookie,
    o.reason, o.created_at, o.updated_at,
    COALESCE(q.quota_5h, 1.0) AS quota_5h,
    COALESCE(q.quota_7d, 1.0) AS quota_7d,
    COALESCE(q.quota_1mo, 1.0) AS quota_1mo,
    COALESCE(q.reset_5h, '') AS reset_5h,
    COALESCE(q.reset_7d, '') AS reset_7d,
    COALESCE(q.reset_1mo, '') AS reset_1mo,
    COALESCE(q.rewards_count, 0) AS rewards_count,
    COALESCE(q.throttled_until, '') AS throttled_until,
    COALESCE(q.synced_at, '') AS synced_at
FROM opencode_go o
LEFT JOIN opencode_go_quota q ON q.credential_id = o.id
ORDER BY o.id;

-- name: ListOpenCodeGoPaged :many
SELECT
    o.id, o.status, o.api_key, o.auth_cookie,
    o.reason, o.created_at, o.updated_at,
    COALESCE(q.quota_5h, 1.0) AS quota_5h,
    COALESCE(q.quota_7d, 1.0) AS quota_7d,
    COALESCE(q.quota_1mo, 1.0) AS quota_1mo,
    COALESCE(q.reset_5h, '') AS reset_5h,
    COALESCE(q.reset_7d, '') AS reset_7d,
    COALESCE(q.reset_1mo, '') AS reset_1mo,
    COALESCE(q.rewards_count, 0) AS rewards_count,
    COALESCE(q.throttled_until, '') AS throttled_until,
    COALESCE(q.synced_at, '') AS synced_at
FROM opencode_go o
LEFT JOIN opencode_go_quota q ON q.credential_id = o.id
WHERE
    (sqlc.arg(search) = '' OR LOWER(o.id) LIKE sqlc.arg(search) OR LOWER(o.status) LIKE sqlc.arg(search))
    AND (
        sqlc.arg(statuses) = ''
        OR (
            (instr(',' || sqlc.arg(statuses) || ',', ',enabled,') = 0 OR o.status = 'enabled')
            AND (instr(',' || sqlc.arg(statuses) || ',', ',disabled,') = 0 OR o.status = 'disabled')
            AND (instr(',' || sqlc.arg(statuses) || ',', ',throttled:all,') = 0 OR q.throttled_until > datetime('now'))
            AND (instr(',' || sqlc.arg(statuses) || ',', ',throttled:default,') = 0 OR q.throttled_until > datetime('now'))
        )
    )
    AND (sqlc.arg(unsynced_only) = 0 OR q.synced_at IS NULL OR q.synced_at = '')
ORDER BY o.id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: UpsertOpenCodeGo :one
INSERT INTO opencode_go (id, status, api_key, auth_cookie, reason, updated_at)
VALUES (?, ?, ?, ?, ?, datetime('now'))
ON CONFLICT (id) DO UPDATE
SET
    status = excluded.status,
    api_key = excluded.api_key,
    auth_cookie = excluded.auth_cookie,
    reason = excluded.reason,
    updated_at = datetime('now')
RETURNING *;

-- name: DeleteOpenCodeGo :execrows
DELETE FROM opencode_go WHERE id = ?;

-- name: UpdateOpenCodeGoStatus :one
UPDATE opencode_go
SET status = ?, reason = ?, updated_at = datetime('now')
WHERE id = ?
RETURNING *;

-- name: RestoreExpiredThrottledOpenCodeGo :exec
UPDATE opencode_go
SET status = 'enabled', reason = '', updated_at = datetime('now')
WHERE status = 'throttled';

-- name: NextOpenCodeGoThrottleDeadline :one
SELECT CAST(COALESCE(MIN(q.throttled_until), '') AS TEXT) AS deadline
FROM opencode_go o
JOIN opencode_go_quota q ON q.credential_id = o.id
WHERE o.status <> 'disabled' AND q.throttled_until > datetime('now');
