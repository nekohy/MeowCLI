-- name: GetOpenCodeGo :one
SELECT *
FROM opencode_go
WHERE id = $1
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
            (strpos(',' || sqlc.arg(statuses) || ',', ',enabled,') = 0 OR o.status = 'enabled')
            AND (strpos(',' || sqlc.arg(statuses) || ',', ',disabled,') = 0 OR o.status = 'disabled')
            AND (strpos(',' || sqlc.arg(statuses) || ',', ',throttled:all,') = 0 OR q.throttled_until > NOW())
            AND (strpos(',' || sqlc.arg(statuses) || ',', ',throttled:default,') = 0 OR q.throttled_until > NOW())
        )
    )
    AND (sqlc.arg(unsynced_only) = FALSE OR q.synced_at IS NULL OR q.synced_at <= '0001-01-01'::timestamptz);

-- name: ListOpenCodeGo :many
SELECT
    o.id, o.status, o.api_key, o.auth_cookie,
    o.reason, o.created_at, o.updated_at,
    COALESCE(q.quota_5h, 1.0)::float8 AS quota_5h,
    COALESCE(q.quota_7d, 1.0)::float8 AS quota_7d,
    COALESCE(q.quota_1mo, 1.0)::float8 AS quota_1mo,
    COALESCE(q.reset_5h, '0001-01-01'::timestamptz) AS reset_5h,
    COALESCE(q.reset_7d, '0001-01-01'::timestamptz) AS reset_7d,
    COALESCE(q.reset_1mo, '0001-01-01'::timestamptz) AS reset_1mo,
    COALESCE(q.rewards_count, 0)::int AS rewards_count,
    COALESCE(q.throttled_until, '0001-01-01'::timestamptz) AS throttled_until,
    COALESCE(q.synced_at, '0001-01-01'::timestamptz) AS synced_at
FROM opencode_go o
LEFT JOIN opencode_go_quota q ON q.credential_id = o.id
ORDER BY o.id;

-- name: ListOpenCodeGoPaged :many
SELECT
    o.id, o.status, o.api_key, o.auth_cookie,
    o.reason, o.created_at, o.updated_at,
    COALESCE(q.quota_5h, 1.0)::float8 AS quota_5h,
    COALESCE(q.quota_7d, 1.0)::float8 AS quota_7d,
    COALESCE(q.quota_1mo, 1.0)::float8 AS quota_1mo,
    COALESCE(q.reset_5h, '0001-01-01'::timestamptz) AS reset_5h,
    COALESCE(q.reset_7d, '0001-01-01'::timestamptz) AS reset_7d,
    COALESCE(q.reset_1mo, '0001-01-01'::timestamptz) AS reset_1mo,
    COALESCE(q.rewards_count, 0)::int AS rewards_count,
    COALESCE(q.throttled_until, '0001-01-01'::timestamptz) AS throttled_until,
    COALESCE(q.synced_at, '0001-01-01'::timestamptz) AS synced_at
FROM opencode_go o
LEFT JOIN opencode_go_quota q ON q.credential_id = o.id
WHERE
    (sqlc.arg(search) = '' OR LOWER(o.id) LIKE sqlc.arg(search) OR LOWER(o.status) LIKE sqlc.arg(search))
    AND (
        sqlc.arg(statuses) = ''
        OR (
            (strpos(',' || sqlc.arg(statuses) || ',', ',enabled,') = 0 OR o.status = 'enabled')
            AND (strpos(',' || sqlc.arg(statuses) || ',', ',disabled,') = 0 OR o.status = 'disabled')
            AND (strpos(',' || sqlc.arg(statuses) || ',', ',throttled:all,') = 0 OR q.throttled_until > NOW())
            AND (strpos(',' || sqlc.arg(statuses) || ',', ',throttled:default,') = 0 OR q.throttled_until > NOW())
        )
    )
    AND (sqlc.arg(unsynced_only) = FALSE OR q.synced_at IS NULL OR q.synced_at <= '0001-01-01'::timestamptz)
ORDER BY o.id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: UpsertOpenCodeGo :one
INSERT INTO opencode_go (id, status, api_key, auth_cookie, reason, updated_at)
VALUES ($1, $2, $3, $4, $5, NOW())
ON CONFLICT (id) DO UPDATE
SET
    status = EXCLUDED.status,
    api_key = EXCLUDED.api_key,
    auth_cookie = EXCLUDED.auth_cookie,
    reason = EXCLUDED.reason,
    updated_at = NOW()
RETURNING *;

-- name: DeleteOpenCodeGo :execrows
DELETE FROM opencode_go WHERE id = $1;

-- name: UpdateOpenCodeGoStatus :one
UPDATE opencode_go
SET status = $1, reason = $2, updated_at = NOW()
WHERE id = $3
RETURNING *;

-- name: RestoreExpiredThrottledOpenCodeGo :exec
UPDATE opencode_go
SET status = 'enabled', reason = '', updated_at = NOW()
WHERE status = 'throttled';

-- name: NextOpenCodeGoThrottleDeadline :one
SELECT MIN(q.throttled_until)::timestamptz AS deadline
FROM opencode_go o
JOIN opencode_go_quota q ON q.credential_id = o.id
WHERE o.status <> 'disabled' AND q.throttled_until > NOW();
