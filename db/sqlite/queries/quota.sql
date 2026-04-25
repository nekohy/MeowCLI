-- name: UpsertQuota :one
-- Syncs remaining quota ratios and reset timestamps from upstream.
INSERT INTO codex_quota (credential_id, quota_5h, quota_7d, quota_spark_5h, quota_spark_7d, reset_5h, reset_7d, reset_spark_5h, reset_spark_7d, synced_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
ON CONFLICT (credential_id) DO UPDATE
SET
    quota_5h       = EXCLUDED.quota_5h,
    quota_7d       = EXCLUDED.quota_7d,
    quota_spark_5h = EXCLUDED.quota_spark_5h,
    quota_spark_7d = EXCLUDED.quota_spark_7d,
    reset_5h       = EXCLUDED.reset_5h,
    reset_7d       = EXCLUDED.reset_7d,
    reset_spark_5h = EXCLUDED.reset_spark_5h,
    reset_spark_7d = EXCLUDED.reset_spark_7d,
    synced_at      = datetime('now')
RETURNING *;

-- name: SetQuotaThrottledAll :exec
-- Sets all tier throttles for a credential (non-tier-specific backoff).
INSERT INTO codex_quota (credential_id, throttled_until, throttled_until_spark, synced_at)
VALUES (?, ?, ?, datetime('now'))
ON CONFLICT (credential_id) DO UPDATE
SET
    throttled_until = excluded.throttled_until,
    throttled_until_spark = excluded.throttled_until_spark,
    synced_at = datetime('now');

-- name: SetQuotaThrottled :exec
-- Sets the default tier throttle for a credential.
INSERT INTO codex_quota (credential_id, throttled_until, synced_at)
VALUES (?, ?, datetime('now'))
ON CONFLICT (credential_id) DO UPDATE
SET
    throttled_until = excluded.throttled_until,
    synced_at = datetime('now');

-- name: SetQuotaThrottledSpark :exec
-- Sets the spark tier throttle for a credential.
INSERT INTO codex_quota (credential_id, throttled_until_spark, synced_at)
VALUES (?, ?, datetime('now'))
ON CONFLICT (credential_id) DO UPDATE
SET
    throttled_until_spark = excluded.throttled_until_spark,
    synced_at = datetime('now');

-- name: GetQuota :one
SELECT * FROM codex_quota WHERE credential_id = ? LIMIT 1;

-- name: ListAvailableCodex :many
-- Returns enabled, non-throttled, non-expired credentials
-- ordered by highest remaining quota ratio first.
SELECT
    c.id,
    c.plan_type,
    COALESCE(q.quota_5h, 1.0)                    AS quota_5h,
    COALESCE(q.quota_7d, 1.0)                    AS quota_7d,
    COALESCE(q.quota_spark_5h, 1.0)              AS quota_spark_5h,
    COALESCE(q.quota_spark_7d, 1.0)              AS quota_spark_7d,
    COALESCE(q.reset_5h, '')                      AS reset_5h,
    COALESCE(q.reset_7d, '')                      AS reset_7d,
    COALESCE(q.reset_spark_5h, '')                AS reset_spark_5h,
    COALESCE(q.reset_spark_7d, '')                AS reset_spark_7d,
    COALESCE(q.throttled_until, datetime('now')) AS throttled_until,
    COALESCE(q.throttled_until_spark, datetime('now')) AS throttled_until_spark,
    COALESCE(q.synced_at, '')                     AS synced_at
FROM codex c
LEFT JOIN codex_quota q ON q.credential_id = c.id
WHERE c.status = 'enabled'
  AND (c.expired > datetime('now') OR c.refresh_token != '')
ORDER BY
    COALESCE(q.quota_5h, 1.0) DESC,
    COALESCE(q.quota_7d, 1.0) DESC;

-- name: DeleteQuota :execrows
-- Deletes the quota record for a credential.
DELETE FROM codex_quota WHERE credential_id = ?;
