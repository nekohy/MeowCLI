-- name: UpsertQuota :one
-- Syncs remaining quota ratios and reset timestamps from upstream.
INSERT INTO codex_quota (credential_id, quota_5h, quota_7d, quota_spark_5h, quota_spark_7d, reset_5h, reset_7d, reset_spark_5h, reset_spark_7d, synced_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
ON CONFLICT (credential_id) DO UPDATE
SET
    quota_5h      = EXCLUDED.quota_5h,
    quota_7d      = EXCLUDED.quota_7d,
    quota_spark_5h = EXCLUDED.quota_spark_5h,
    quota_spark_7d = EXCLUDED.quota_spark_7d,
    reset_5h      = EXCLUDED.reset_5h,
    reset_7d      = EXCLUDED.reset_7d,
    reset_spark_5h = EXCLUDED.reset_spark_5h,
    reset_spark_7d = EXCLUDED.reset_spark_7d,
    synced_at     = NOW()
RETURNING *;

-- name: SetQuotaThrottledAll :exec
-- Sets all tier throttles for a credential (non-tier-specific backoff).
INSERT INTO codex_quota (credential_id, throttled_until, throttled_until_spark, synced_at)
VALUES ($1, $2, $2, NOW())
ON CONFLICT (credential_id) DO UPDATE
SET
    throttled_until = EXCLUDED.throttled_until,
    throttled_until_spark = EXCLUDED.throttled_until_spark,
    synced_at = NOW();

-- name: SetQuotaThrottled :exec
-- Sets the default tier throttle for a credential.
INSERT INTO codex_quota (credential_id, throttled_until, synced_at)
VALUES ($1, $2, NOW())
ON CONFLICT (credential_id) DO UPDATE
SET
    throttled_until = EXCLUDED.throttled_until,
    synced_at = NOW();

-- name: SetQuotaThrottledSpark :exec
-- Sets the spark tier throttle for a credential.
INSERT INTO codex_quota (credential_id, throttled_until_spark, synced_at)
VALUES ($1, $2, NOW())
ON CONFLICT (credential_id) DO UPDATE
SET
    throttled_until_spark = EXCLUDED.throttled_until_spark,
    synced_at = NOW();

-- name: ClearQuotaThrottle :exec
-- Clears all tier throttles for a credential by moving them to now.
INSERT INTO codex_quota (credential_id, throttled_until, throttled_until_spark, synced_at)
VALUES ($1, NOW(), NOW(), NOW())
ON CONFLICT (credential_id) DO UPDATE
SET
    throttled_until = NOW(),
    throttled_until_spark = NOW(),
    synced_at = NOW();

-- name: GetQuota :one
SELECT * FROM codex_quota WHERE credential_id = $1 LIMIT 1;

-- name: ListAvailableCodex :many
-- Returns enabled, non-throttled, non-expired credentials
-- ordered by highest remaining quota ratio first.
SELECT
    c.id,
    c.plan_type,
    COALESCE(q.quota_5h, 1.0)          AS quota_5h,
    COALESCE(q.quota_7d, 1.0)          AS quota_7d,
    COALESCE(q.quota_spark_5h, 1.0)    AS quota_spark_5h,
    COALESCE(q.quota_spark_7d, 1.0)    AS quota_spark_7d,
    COALESCE(q.reset_5h, '0001-01-01'::timestamptz)        AS reset_5h,
    COALESCE(q.reset_7d, '0001-01-01'::timestamptz)        AS reset_7d,
    COALESCE(q.reset_spark_5h, '0001-01-01'::timestamptz)  AS reset_spark_5h,
    COALESCE(q.reset_spark_7d, '0001-01-01'::timestamptz)  AS reset_spark_7d,
    COALESCE(q.throttled_until, NOW()) AS throttled_until,
    COALESCE(q.throttled_until_spark, NOW()) AS throttled_until_spark,
    COALESCE(q.synced_at, '0001-01-01'::timestamptz) AS synced_at
FROM codex c
LEFT JOIN codex_quota q ON q.credential_id = c.id
WHERE c.status = 'enabled'
  AND (c.expired > NOW() OR c.refresh_token != '')
ORDER BY
    COALESCE(q.quota_5h, 1.0) DESC,
    COALESCE(q.quota_7d, 1.0) DESC;

-- name: DeleteQuota :execrows
-- Deletes the quota record for a credential.
DELETE FROM codex_quota WHERE credential_id = $1;
