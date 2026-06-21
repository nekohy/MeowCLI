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
    reason = ''
WHERE id = $1
RETURNING *;

-- name: UpdateCodexPlanType :one
UPDATE codex
SET
    plan_type = $1,
    reason = ''
WHERE id = $2
RETURNING *;

-- name: CountCodex :one
SELECT COUNT(*) FROM codex;

-- name: CountCodexFiltered :one
SELECT COUNT(*)
FROM codex c
LEFT JOIN codex_quota q ON q.credential_id = c.id
WHERE
    (sqlc.arg(search) = '' OR LOWER(c.id) LIKE sqlc.arg(search) OR LOWER(c.status) LIKE sqlc.arg(search) OR LOWER(c.plan_type) LIKE sqlc.arg(search))
    AND (
        sqlc.arg(statuses) = ''
        OR (
            (strpos(',' || sqlc.arg(statuses) || ',', ',enabled,') = 0 OR c.status = 'enabled')
            AND (strpos(',' || sqlc.arg(statuses) || ',', ',disabled,') = 0 OR c.status = 'disabled')
            AND (strpos(',' || sqlc.arg(statuses) || ',', ',throttled:all,') = 0 OR q.throttled_until > NOW() OR q.throttled_until_spark > NOW())
            AND (strpos(',' || sqlc.arg(statuses) || ',', ',throttled:default,') = 0 OR q.throttled_until > NOW())
            AND (strpos(',' || sqlc.arg(statuses) || ',', ',throttled:spark,') = 0 OR q.throttled_until_spark > NOW())
        )
    )
    AND (sqlc.arg(plan_type) = '' OR LOWER(c.plan_type) = LOWER(sqlc.arg(plan_type)))
    AND ((NOT sqlc.arg(unsynced_only)::boolean) OR q.synced_at IS NULL OR q.synced_at <= '0001-01-01'::timestamptz);

-- name: ListCodexPlanTypes :many
SELECT DISTINCT LOWER(TRIM(c.plan_type)) AS plan_type
FROM codex c
LEFT JOIN codex_quota q ON q.credential_id = c.id
WHERE
    (sqlc.arg(search) = '' OR LOWER(c.id) LIKE sqlc.arg(search) OR LOWER(c.status) LIKE sqlc.arg(search) OR LOWER(c.plan_type) LIKE sqlc.arg(search))
    AND (
        sqlc.arg(statuses) = ''
        OR (
            (strpos(',' || sqlc.arg(statuses) || ',', ',enabled,') = 0 OR c.status = 'enabled')
            AND (strpos(',' || sqlc.arg(statuses) || ',', ',disabled,') = 0 OR c.status = 'disabled')
            AND (strpos(',' || sqlc.arg(statuses) || ',', ',throttled:all,') = 0 OR q.throttled_until > NOW() OR q.throttled_until_spark > NOW())
            AND (strpos(',' || sqlc.arg(statuses) || ',', ',throttled:default,') = 0 OR q.throttled_until > NOW())
            AND (strpos(',' || sqlc.arg(statuses) || ',', ',throttled:spark,') = 0 OR q.throttled_until_spark > NOW())
        )
    )
    AND ((NOT sqlc.arg(unsynced_only)::boolean) OR q.synced_at IS NULL OR q.synced_at <= '0001-01-01'::timestamptz)
    AND TRIM(c.plan_type) <> ''
ORDER BY plan_type;

-- name: ListCodex :many
SELECT
    c.*,
    COALESCE(q.quota_5h, 1.0) AS quota_5h,
    COALESCE(q.quota_7d, 1.0) AS quota_7d,
    COALESCE(q.quota_1mo, 1.0) AS quota_1mo,
    COALESCE(q.quota_spark_5h, 1.0) AS quota_spark_5h,
    COALESCE(q.quota_spark_7d, 1.0) AS quota_spark_7d,
    COALESCE(q.quota_spark_1mo, 1.0) AS quota_spark_1mo,
    COALESCE(q.reset_5h, '0001-01-01'::timestamptz) AS reset_5h,
    COALESCE(q.reset_7d, '0001-01-01'::timestamptz) AS reset_7d,
    COALESCE(q.reset_1mo, '0001-01-01'::timestamptz) AS reset_1mo,
    COALESCE(q.reset_spark_5h, '0001-01-01'::timestamptz) AS reset_spark_5h,
    COALESCE(q.reset_spark_7d, '0001-01-01'::timestamptz) AS reset_spark_7d,
    COALESCE(q.reset_spark_1mo, '0001-01-01'::timestamptz) AS reset_spark_1mo,
    COALESCE(q.throttled_until, '0001-01-01'::timestamptz) AS throttled_until_default,
    COALESCE(q.throttled_until_spark, '0001-01-01'::timestamptz) AS throttled_until_spark,
    COALESCE(q.synced_at, '0001-01-01'::timestamptz) AS synced_at
FROM codex c
LEFT JOIN codex_quota q ON q.credential_id = c.id
ORDER BY c.id;

-- name: ListCodexPaged :many
SELECT
    c.*,
    COALESCE(q.quota_5h, 1.0) AS quota_5h,
    COALESCE(q.quota_7d, 1.0) AS quota_7d,
    COALESCE(q.quota_1mo, 1.0) AS quota_1mo,
    COALESCE(q.quota_spark_5h, 1.0) AS quota_spark_5h,
    COALESCE(q.quota_spark_7d, 1.0) AS quota_spark_7d,
    COALESCE(q.quota_spark_1mo, 1.0) AS quota_spark_1mo,
    COALESCE(q.reset_5h, '0001-01-01'::timestamptz) AS reset_5h,
    COALESCE(q.reset_7d, '0001-01-01'::timestamptz) AS reset_7d,
    COALESCE(q.reset_1mo, '0001-01-01'::timestamptz) AS reset_1mo,
    COALESCE(q.reset_spark_5h, '0001-01-01'::timestamptz) AS reset_spark_5h,
    COALESCE(q.reset_spark_7d, '0001-01-01'::timestamptz) AS reset_spark_7d,
    COALESCE(q.reset_spark_1mo, '0001-01-01'::timestamptz) AS reset_spark_1mo,
    COALESCE(q.throttled_until, '0001-01-01'::timestamptz) AS throttled_until_default,
    COALESCE(q.throttled_until_spark, '0001-01-01'::timestamptz) AS throttled_until_spark,
    COALESCE(q.synced_at, '0001-01-01'::timestamptz) AS synced_at
FROM codex c
LEFT JOIN codex_quota q ON q.credential_id = c.id
WHERE
    (sqlc.arg(search) = '' OR LOWER(c.id) LIKE sqlc.arg(search) OR LOWER(c.status) LIKE sqlc.arg(search) OR LOWER(c.plan_type) LIKE sqlc.arg(search))
    AND (
        sqlc.arg(statuses) = ''
        OR (
            (strpos(',' || sqlc.arg(statuses) || ',', ',enabled,') = 0 OR c.status = 'enabled')
            AND (strpos(',' || sqlc.arg(statuses) || ',', ',disabled,') = 0 OR c.status = 'disabled')
            AND (strpos(',' || sqlc.arg(statuses) || ',', ',throttled:all,') = 0 OR q.throttled_until > NOW() OR q.throttled_until_spark > NOW())
            AND (strpos(',' || sqlc.arg(statuses) || ',', ',throttled:default,') = 0 OR q.throttled_until > NOW())
            AND (strpos(',' || sqlc.arg(statuses) || ',', ',throttled:spark,') = 0 OR q.throttled_until_spark > NOW())
        )
    )
    AND (sqlc.arg(plan_type) = '' OR LOWER(c.plan_type) = LOWER(sqlc.arg(plan_type)))
    AND ((NOT sqlc.arg(unsynced_only)::boolean) OR q.synced_at IS NULL OR q.synced_at <= '0001-01-01'::timestamptz)
ORDER BY c.id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CreateCodex :one
INSERT INTO codex (id, status, access_token, expired, refresh_token, plan_type)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: DeleteCodex :execrows
DELETE FROM codex WHERE id = $1;

-- name: UpdateCodexStatus :one
UPDATE codex SET status = $2, reason = $3 WHERE id = $1
RETURNING *;

-- name: RestoreExpiredThrottledCodex :exec
UPDATE codex
SET status = 'enabled', reason = ''
WHERE status = 'throttled';

-- name: NextCodexThrottleDeadline :one
SELECT MIN(deadline)::timestamptz AS deadline
FROM (
    SELECT q.throttled_until AS deadline
    FROM codex c
    JOIN codex_quota q ON q.credential_id = c.id
    WHERE c.status <> 'disabled' AND q.throttled_until > NOW()
    UNION ALL
    SELECT q.throttled_until_spark AS deadline
    FROM codex c
    JOIN codex_quota q ON q.credential_id = c.id
    WHERE c.status <> 'disabled' AND q.throttled_until_spark > NOW()
) deadlines;
