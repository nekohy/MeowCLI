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

-- name: UpdateCodexPlanType :one
UPDATE codex
SET
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
    AND (
        sqlc.arg(statuses) = ''
        OR (
            (instr(',' || sqlc.arg(statuses) || ',', ',enabled,') = 0 OR c.status = 'enabled')
            AND (instr(',' || sqlc.arg(statuses) || ',', ',disabled,') = 0 OR c.status = 'disabled')
            AND (instr(',' || sqlc.arg(statuses) || ',', ',throttled:all,') = 0 OR q.throttled_until > datetime('now') OR q.throttled_until_spark > datetime('now'))
            AND (instr(',' || sqlc.arg(statuses) || ',', ',throttled:default,') = 0 OR q.throttled_until > datetime('now'))
            AND (instr(',' || sqlc.arg(statuses) || ',', ',throttled:spark,') = 0 OR q.throttled_until_spark > datetime('now'))
        )
    )
    AND (sqlc.arg(plan_type) = '' OR LOWER(c.plan_type) = LOWER(sqlc.arg(plan_type)))
    AND (sqlc.arg(unsynced_only) = 0 OR q.synced_at IS NULL OR q.synced_at = '');

-- name: ListCodexPlanTypes :many
SELECT DISTINCT LOWER(TRIM(c.plan_type)) AS plan_type
FROM codex c
LEFT JOIN codex_quota q ON q.credential_id = c.id
WHERE
    (sqlc.arg(search) = '' OR LOWER(c.id) LIKE sqlc.arg(search) OR LOWER(c.status) LIKE sqlc.arg(search) OR LOWER(c.plan_type) LIKE sqlc.arg(search))
    AND (
        sqlc.arg(statuses) = ''
        OR (
            (instr(',' || sqlc.arg(statuses) || ',', ',enabled,') = 0 OR c.status = 'enabled')
            AND (instr(',' || sqlc.arg(statuses) || ',', ',disabled,') = 0 OR c.status = 'disabled')
            AND (instr(',' || sqlc.arg(statuses) || ',', ',throttled:all,') = 0 OR q.throttled_until > datetime('now') OR q.throttled_until_spark > datetime('now'))
            AND (instr(',' || sqlc.arg(statuses) || ',', ',throttled:default,') = 0 OR q.throttled_until > datetime('now'))
            AND (instr(',' || sqlc.arg(statuses) || ',', ',throttled:spark,') = 0 OR q.throttled_until_spark > datetime('now'))
        )
    )
    AND (sqlc.arg(unsynced_only) = 0 OR q.synced_at IS NULL OR q.synced_at = '')
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
    COALESCE(q.reset_5h, '') AS reset_5h,
    COALESCE(q.reset_7d, '') AS reset_7d,
    COALESCE(q.reset_1mo, '') AS reset_1mo,
    COALESCE(q.reset_spark_5h, '') AS reset_spark_5h,
    COALESCE(q.reset_spark_7d, '') AS reset_spark_7d,
    COALESCE(q.reset_spark_1mo, '') AS reset_spark_1mo,
    COALESCE(q.reset_credits_count, 0) AS reset_credits_count,
    COALESCE(q.throttled_until, '') AS throttled_until_default,
    COALESCE(q.throttled_until_spark, '') AS throttled_until_spark,
    COALESCE(q.synced_at, '') AS synced_at
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
    COALESCE(q.reset_5h, '') AS reset_5h,
    COALESCE(q.reset_7d, '') AS reset_7d,
    COALESCE(q.reset_1mo, '') AS reset_1mo,
    COALESCE(q.reset_spark_5h, '') AS reset_spark_5h,
    COALESCE(q.reset_spark_7d, '') AS reset_spark_7d,
    COALESCE(q.reset_spark_1mo, '') AS reset_spark_1mo,
    COALESCE(q.reset_credits_count, 0) AS reset_credits_count,
    COALESCE(q.throttled_until, '') AS throttled_until_default,
    COALESCE(q.throttled_until_spark, '') AS throttled_until_spark,
    COALESCE(q.synced_at, '') AS synced_at
FROM codex c
LEFT JOIN codex_quota q ON q.credential_id = c.id
WHERE
    (sqlc.arg(search) = '' OR LOWER(c.id) LIKE sqlc.arg(search) OR LOWER(c.status) LIKE sqlc.arg(search) OR LOWER(c.plan_type) LIKE sqlc.arg(search))
    AND (
        sqlc.arg(statuses) = ''
        OR (
            (instr(',' || sqlc.arg(statuses) || ',', ',enabled,') = 0 OR c.status = 'enabled')
            AND (instr(',' || sqlc.arg(statuses) || ',', ',disabled,') = 0 OR c.status = 'disabled')
            AND (instr(',' || sqlc.arg(statuses) || ',', ',throttled:all,') = 0 OR q.throttled_until > datetime('now') OR q.throttled_until_spark > datetime('now'))
            AND (instr(',' || sqlc.arg(statuses) || ',', ',throttled:default,') = 0 OR q.throttled_until > datetime('now'))
            AND (instr(',' || sqlc.arg(statuses) || ',', ',throttled:spark,') = 0 OR q.throttled_until_spark > datetime('now'))
        )
    )
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

-- name: RestoreExpiredThrottledCodex :exec
UPDATE codex
SET status = 'enabled', reason = ''
WHERE status = 'throttled';

-- name: NextCodexThrottleDeadline :one
SELECT CAST(COALESCE(MIN(deadline), '') AS TEXT) AS deadline
FROM (
    SELECT q.throttled_until AS deadline
    FROM codex c
    JOIN codex_quota q ON q.credential_id = c.id
    WHERE c.status <> 'disabled' AND q.throttled_until > datetime('now')
    UNION ALL
    SELECT q.throttled_until_spark AS deadline
    FROM codex c
    JOIN codex_quota q ON q.credential_id = c.id
    WHERE c.status <> 'disabled' AND q.throttled_until_spark > datetime('now')
);
