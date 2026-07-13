-- name: UpsertOpenCodeGoQuota :one
INSERT INTO opencode_go_quota (
    credential_id, quota_5h, quota_7d, quota_1mo,
    reset_5h, reset_7d, reset_1mo,
    rewards_count, synced_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
ON CONFLICT (credential_id) DO UPDATE
SET
    quota_5h = excluded.quota_5h,
    quota_7d = excluded.quota_7d,
    quota_1mo = excluded.quota_1mo,
    reset_5h = excluded.reset_5h,
    reset_7d = excluded.reset_7d,
    reset_1mo = excluded.reset_1mo,
    rewards_count = excluded.rewards_count,
    synced_at = datetime('now')
RETURNING *;

-- name: SetOpenCodeGoQuotaThrottled :exec
INSERT INTO opencode_go_quota (credential_id, throttled_until, synced_at)
VALUES (?, ?, datetime('now'))
ON CONFLICT (credential_id) DO UPDATE
SET throttled_until = excluded.throttled_until;

-- name: ClearOpenCodeGoQuotaThrottle :exec
INSERT INTO opencode_go_quota (credential_id, throttled_until, synced_at)
VALUES (?, datetime('now'), datetime('now'))
ON CONFLICT (credential_id) DO UPDATE
SET throttled_until = datetime('now');

-- name: DeleteOpenCodeGoQuota :execrows
DELETE FROM opencode_go_quota WHERE credential_id = ?;

-- name: ListAvailableOpenCodeGo :many
SELECT
    o.id,
    o.auth_cookie,
    COALESCE(q.quota_5h, 1.0) AS quota_5h,
    COALESCE(q.quota_7d, 1.0) AS quota_7d,
    COALESCE(q.quota_1mo, 1.0) AS quota_1mo,
    COALESCE(q.reset_5h, '') AS reset_5h,
    COALESCE(q.reset_7d, '') AS reset_7d,
    COALESCE(q.reset_1mo, '') AS reset_1mo,
    COALESCE(q.rewards_count, 0) AS rewards_count,
    COALESCE(q.throttled_until, '') AS throttled_until,
    COALESCE(q.synced_at, o.created_at) AS synced_at
FROM opencode_go o
LEFT JOIN opencode_go_quota q ON q.credential_id = o.id
WHERE o.status = 'enabled'
ORDER BY
    COALESCE(q.quota_5h, 1.0) DESC,
    COALESCE(q.quota_7d, 1.0) DESC,
    COALESCE(q.quota_1mo, 1.0) DESC,
    o.id;
