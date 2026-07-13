-- name: UpsertOpenCodeGoQuota :one
INSERT INTO opencode_go_quota (
    credential_id, quota_5h, quota_7d, quota_1mo,
    reset_5h, reset_7d, reset_1mo,
    rewards_count, synced_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
ON CONFLICT (credential_id) DO UPDATE
SET
    quota_5h = EXCLUDED.quota_5h,
    quota_7d = EXCLUDED.quota_7d,
    quota_1mo = EXCLUDED.quota_1mo,
    reset_5h = EXCLUDED.reset_5h,
    reset_7d = EXCLUDED.reset_7d,
    reset_1mo = EXCLUDED.reset_1mo,
    rewards_count = EXCLUDED.rewards_count,
    synced_at = NOW()
RETURNING *;

-- name: SetOpenCodeGoQuotaThrottled :exec
INSERT INTO opencode_go_quota (credential_id, throttled_until, synced_at)
VALUES ($1, $2, NOW())
ON CONFLICT (credential_id) DO UPDATE
SET throttled_until = EXCLUDED.throttled_until;

-- name: ClearOpenCodeGoQuotaThrottle :exec
INSERT INTO opencode_go_quota (credential_id, throttled_until, synced_at)
VALUES ($1, NOW(), NOW())
ON CONFLICT (credential_id) DO UPDATE
SET throttled_until = NOW();

-- name: DeleteOpenCodeGoQuota :execrows
DELETE FROM opencode_go_quota WHERE credential_id = $1;

-- name: ListAvailableOpenCodeGo :many
SELECT
    o.id,
    o.auth_cookie,
    COALESCE(q.quota_5h, 1.0)::float8 AS quota_5h,
    COALESCE(q.quota_7d, 1.0)::float8 AS quota_7d,
    COALESCE(q.quota_1mo, 1.0)::float8 AS quota_1mo,
    COALESCE(q.reset_5h, '0001-01-01'::timestamptz) AS reset_5h,
    COALESCE(q.reset_7d, '0001-01-01'::timestamptz) AS reset_7d,
    COALESCE(q.reset_1mo, '0001-01-01'::timestamptz) AS reset_1mo,
    COALESCE(q.rewards_count, 0)::int AS rewards_count,
    COALESCE(q.throttled_until, '0001-01-01'::timestamptz) AS throttled_until,
    COALESCE(q.synced_at, o.created_at) AS synced_at
FROM opencode_go o
LEFT JOIN opencode_go_quota q ON q.credential_id = o.id
WHERE o.status = 'enabled'
ORDER BY
    COALESCE(q.quota_5h, 1.0) DESC,
    COALESCE(q.quota_7d, 1.0) DESC,
    COALESCE(q.quota_1mo, 1.0) DESC,
    o.id;
