-- name: GetAntigravity :one
SELECT *
FROM antigravity
WHERE id = ?
LIMIT 1;

-- name: CountEnabledAntigravity :one
SELECT COUNT(*)
FROM antigravity
WHERE status = 'enabled';

-- name: CountAntigravity :one
SELECT COUNT(*) FROM antigravity;

-- name: CountAntigravityFiltered :one
SELECT COUNT(*)
FROM antigravity a
LEFT JOIN antigravity_quota q ON q.credential_id = a.id
WHERE
    (sqlc.arg(search) = '' OR LOWER(a.id) LIKE sqlc.arg(search) OR LOWER(a.email) LIKE sqlc.arg(search) OR LOWER(a.status) LIKE sqlc.arg(search) OR LOWER(a.plan_type) LIKE sqlc.arg(search))
    AND (
        sqlc.arg(statuses) = ''
        OR (
            (instr(',' || sqlc.arg(statuses) || ',', ',enabled,') = 0 OR a.status = 'enabled')
            AND (instr(',' || sqlc.arg(statuses) || ',', ',disabled,') = 0 OR a.status = 'disabled')
            AND (instr(',' || sqlc.arg(statuses) || ',', ',throttled:all,') = 0 OR q.throttled_until_claude > datetime('now') OR q.throttled_until_pro > datetime('now') OR q.throttled_until_flash > datetime('now') OR q.throttled_until_flashlite > datetime('now') OR q.throttled_until_tab > datetime('now') OR q.throttled_until_image > datetime('now'))
            AND (instr(',' || sqlc.arg(statuses) || ',', ',throttled:claude,') = 0 OR q.throttled_until_claude > datetime('now'))
            AND (instr(',' || sqlc.arg(statuses) || ',', ',throttled:pro,') = 0 OR q.throttled_until_pro > datetime('now'))
            AND (instr(',' || sqlc.arg(statuses) || ',', ',throttled:flash,') = 0 OR q.throttled_until_flash > datetime('now'))
            AND (instr(',' || sqlc.arg(statuses) || ',', ',throttled:flashlite,') = 0 OR q.throttled_until_flashlite > datetime('now'))
            AND (instr(',' || sqlc.arg(statuses) || ',', ',throttled:tab,') = 0 OR q.throttled_until_tab > datetime('now'))
            AND (instr(',' || sqlc.arg(statuses) || ',', ',throttled:image,') = 0 OR q.throttled_until_image > datetime('now'))
        )
    )
    AND (sqlc.arg(plan_type) = '' OR LOWER(a.plan_type) = LOWER(sqlc.arg(plan_type)))
    AND (sqlc.arg(unsynced_only) = 0 OR q.synced_at IS NULL OR q.synced_at = '');

-- name: ListAntigravityPlanTypes :many
SELECT DISTINCT LOWER(TRIM(a.plan_type)) AS plan_type
FROM antigravity a
LEFT JOIN antigravity_quota q ON q.credential_id = a.id
WHERE
    (sqlc.arg(search) = '' OR LOWER(a.id) LIKE sqlc.arg(search) OR LOWER(a.email) LIKE sqlc.arg(search) OR LOWER(a.status) LIKE sqlc.arg(search) OR LOWER(a.plan_type) LIKE sqlc.arg(search))
    AND (
        sqlc.arg(statuses) = ''
        OR (
            (instr(',' || sqlc.arg(statuses) || ',', ',enabled,') = 0 OR a.status = 'enabled')
            AND (instr(',' || sqlc.arg(statuses) || ',', ',disabled,') = 0 OR a.status = 'disabled')
            AND (instr(',' || sqlc.arg(statuses) || ',', ',throttled:all,') = 0 OR q.throttled_until_claude > datetime('now') OR q.throttled_until_pro > datetime('now') OR q.throttled_until_flash > datetime('now') OR q.throttled_until_flashlite > datetime('now') OR q.throttled_until_tab > datetime('now') OR q.throttled_until_image > datetime('now'))
            AND (instr(',' || sqlc.arg(statuses) || ',', ',throttled:claude,') = 0 OR q.throttled_until_claude > datetime('now'))
            AND (instr(',' || sqlc.arg(statuses) || ',', ',throttled:pro,') = 0 OR q.throttled_until_pro > datetime('now'))
            AND (instr(',' || sqlc.arg(statuses) || ',', ',throttled:flash,') = 0 OR q.throttled_until_flash > datetime('now'))
            AND (instr(',' || sqlc.arg(statuses) || ',', ',throttled:flashlite,') = 0 OR q.throttled_until_flashlite > datetime('now'))
            AND (instr(',' || sqlc.arg(statuses) || ',', ',throttled:tab,') = 0 OR q.throttled_until_tab > datetime('now'))
            AND (instr(',' || sqlc.arg(statuses) || ',', ',throttled:image,') = 0 OR q.throttled_until_image > datetime('now'))
        )
    )
    AND (sqlc.arg(unsynced_only) = 0 OR q.synced_at IS NULL OR q.synced_at = '')
    AND TRIM(a.plan_type) <> ''
ORDER BY plan_type;

-- name: ListAntigravity :many
SELECT
    a.id, a.status, a.access_token, a.refresh_token, a.expired,
    a.email, a.project_id, a.plan_type, a.reason,
    COALESCE(q.quota_claude, 1.0) AS quota_claude,
    COALESCE(q.reset_claude, datetime('now')) AS reset_claude,
    COALESCE(q.quota_pro, 1.0) AS quota_pro,
    COALESCE(q.reset_pro, datetime('now')) AS reset_pro,
    COALESCE(q.quota_flash, 1.0) AS quota_flash,
    COALESCE(q.reset_flash, datetime('now')) AS reset_flash,
    COALESCE(q.quota_flashlite, 1.0) AS quota_flashlite,
    COALESCE(q.reset_flashlite, datetime('now')) AS reset_flashlite,
    COALESCE(q.quota_tab, 1.0) AS quota_tab,
    COALESCE(q.reset_tab, datetime('now', '+100 years')) AS reset_tab,
    COALESCE(q.quota_image, 1.0) AS quota_image,
    COALESCE(q.reset_image, datetime('now')) AS reset_image,
    COALESCE(c.credits_amount, 0) AS credits_amount,
    COALESCE(c.credit_types, '') AS credit_types,
    COALESCE(q.throttled_until_claude, '') AS throttled_until_claude,
    COALESCE(q.throttled_until_pro, '') AS throttled_until_pro,
    COALESCE(q.throttled_until_flash, '') AS throttled_until_flash,
    COALESCE(q.throttled_until_flashlite, '') AS throttled_until_flashlite,
    COALESCE(q.throttled_until_tab, '') AS throttled_until_tab,
    COALESCE(q.throttled_until_image, '') AS throttled_until_image,
    COALESCE(q.synced_at, '') AS synced_at
FROM antigravity a
LEFT JOIN antigravity_quota q ON q.credential_id = a.id
LEFT JOIN antigravity_credits c ON c.credential_id = a.id
ORDER BY a.id;

-- name: ListAntigravityPaged :many
SELECT
    a.id, a.status, a.access_token, a.refresh_token, a.expired,
    a.email, a.project_id, a.plan_type, a.reason,
    COALESCE(q.quota_claude, 1.0) AS quota_claude,
    COALESCE(q.reset_claude, datetime('now')) AS reset_claude,
    COALESCE(q.quota_pro, 1.0) AS quota_pro,
    COALESCE(q.reset_pro, datetime('now')) AS reset_pro,
    COALESCE(q.quota_flash, 1.0) AS quota_flash,
    COALESCE(q.reset_flash, datetime('now')) AS reset_flash,
    COALESCE(q.quota_flashlite, 1.0) AS quota_flashlite,
    COALESCE(q.reset_flashlite, datetime('now')) AS reset_flashlite,
    COALESCE(q.quota_tab, 1.0) AS quota_tab,
    COALESCE(q.reset_tab, datetime('now', '+100 years')) AS reset_tab,
    COALESCE(q.quota_image, 1.0) AS quota_image,
    COALESCE(q.reset_image, datetime('now')) AS reset_image,
    COALESCE(c.credits_amount, 0) AS credits_amount,
    COALESCE(c.credit_types, '') AS credit_types,
    COALESCE(q.throttled_until_claude, '') AS throttled_until_claude,
    COALESCE(q.throttled_until_pro, '') AS throttled_until_pro,
    COALESCE(q.throttled_until_flash, '') AS throttled_until_flash,
    COALESCE(q.throttled_until_flashlite, '') AS throttled_until_flashlite,
    COALESCE(q.throttled_until_tab, '') AS throttled_until_tab,
    COALESCE(q.throttled_until_image, '') AS throttled_until_image,
    COALESCE(q.synced_at, '') AS synced_at
FROM antigravity a
LEFT JOIN antigravity_quota q ON q.credential_id = a.id
LEFT JOIN antigravity_credits c ON c.credential_id = a.id
WHERE
    (sqlc.arg(search) = '' OR LOWER(a.id) LIKE sqlc.arg(search) OR LOWER(a.email) LIKE sqlc.arg(search) OR LOWER(a.status) LIKE sqlc.arg(search) OR LOWER(a.plan_type) LIKE sqlc.arg(search))
    AND (
        sqlc.arg(statuses) = ''
        OR (
            (instr(',' || sqlc.arg(statuses) || ',', ',enabled,') = 0 OR a.status = 'enabled')
            AND (instr(',' || sqlc.arg(statuses) || ',', ',disabled,') = 0 OR a.status = 'disabled')
            AND (instr(',' || sqlc.arg(statuses) || ',', ',throttled:all,') = 0 OR q.throttled_until_claude > datetime('now') OR q.throttled_until_pro > datetime('now') OR q.throttled_until_flash > datetime('now') OR q.throttled_until_flashlite > datetime('now') OR q.throttled_until_tab > datetime('now') OR q.throttled_until_image > datetime('now'))
            AND (instr(',' || sqlc.arg(statuses) || ',', ',throttled:claude,') = 0 OR q.throttled_until_claude > datetime('now'))
            AND (instr(',' || sqlc.arg(statuses) || ',', ',throttled:pro,') = 0 OR q.throttled_until_pro > datetime('now'))
            AND (instr(',' || sqlc.arg(statuses) || ',', ',throttled:flash,') = 0 OR q.throttled_until_flash > datetime('now'))
            AND (instr(',' || sqlc.arg(statuses) || ',', ',throttled:flashlite,') = 0 OR q.throttled_until_flashlite > datetime('now'))
            AND (instr(',' || sqlc.arg(statuses) || ',', ',throttled:tab,') = 0 OR q.throttled_until_tab > datetime('now'))
            AND (instr(',' || sqlc.arg(statuses) || ',', ',throttled:image,') = 0 OR q.throttled_until_image > datetime('now'))
        )
    )
    AND (sqlc.arg(plan_type) = '' OR LOWER(a.plan_type) = LOWER(sqlc.arg(plan_type)))
    AND (sqlc.arg(unsynced_only) = 0 OR q.synced_at IS NULL OR q.synced_at = '')
ORDER BY a.id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: UpsertAntigravity :one
INSERT INTO antigravity (id, status, access_token, refresh_token, expired, email, project_id, plan_type, reason, synced_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
ON CONFLICT (id) DO UPDATE
SET
    status = EXCLUDED.status,
    access_token = EXCLUDED.access_token,
    refresh_token = EXCLUDED.refresh_token,
    expired = EXCLUDED.expired,
    email = EXCLUDED.email,
    project_id = EXCLUDED.project_id,
    plan_type = EXCLUDED.plan_type,
    reason = EXCLUDED.reason,
    synced_at = datetime('now')
RETURNING *;

-- name: UpdateAntigravityTokens :one
UPDATE antigravity
SET
    status = ?,
    access_token = ?,
    refresh_token = ?,
    expired = ?,
    email = ?,
    project_id = ?,
    plan_type = ?,
    reason = '',
    synced_at = datetime('now')
WHERE id = ?
RETURNING *;

-- name: DeleteAntigravity :execrows
DELETE FROM antigravity WHERE id = ?;

-- name: UpdateAntigravityStatus :one
UPDATE antigravity
SET status = ?, reason = ?
WHERE id = ?
RETURNING *;

-- name: RestoreExpiredThrottledAntigravity :exec
UPDATE antigravity
SET status = 'enabled', reason = ''
WHERE status = 'throttled';

-- name: NextAntigravityThrottleDeadline :one
SELECT CAST(COALESCE(MIN(deadline), '') AS TEXT) AS deadline
FROM (
    SELECT q.throttled_until_claude AS deadline
    FROM antigravity a
    JOIN antigravity_quota q ON q.credential_id = a.id
    WHERE a.status <> 'disabled' AND q.throttled_until_claude > datetime('now')
    UNION ALL
    SELECT q.throttled_until_pro AS deadline
    FROM antigravity a
    JOIN antigravity_quota q ON q.credential_id = a.id
    WHERE a.status <> 'disabled' AND q.throttled_until_pro > datetime('now')
    UNION ALL
    SELECT q.throttled_until_flash AS deadline
    FROM antigravity a
    JOIN antigravity_quota q ON q.credential_id = a.id
    WHERE a.status <> 'disabled' AND q.throttled_until_flash > datetime('now')
    UNION ALL
    SELECT q.throttled_until_flashlite AS deadline
    FROM antigravity a
    JOIN antigravity_quota q ON q.credential_id = a.id
    WHERE a.status <> 'disabled' AND q.throttled_until_flashlite > datetime('now')
    UNION ALL
    SELECT q.throttled_until_tab AS deadline
    FROM antigravity a
    JOIN antigravity_quota q ON q.credential_id = a.id
    WHERE a.status <> 'disabled' AND q.throttled_until_tab > datetime('now')
    UNION ALL
    SELECT q.throttled_until_image AS deadline
    FROM antigravity a
    JOIN antigravity_quota q ON q.credential_id = a.id
    WHERE a.status <> 'disabled' AND q.throttled_until_image > datetime('now')
);
