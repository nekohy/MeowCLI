-- name: CountAuthKeys :one
SELECT COUNT(*) FROM auth_keys;

-- name: ListAuthKeys :many
SELECT key, role, note, created_at FROM auth_keys ORDER BY created_at;

-- name: GetAuthKey :one
SELECT key, role, note, created_at FROM auth_keys WHERE key = ?;

-- name: CreateAuthKey :one
INSERT INTO auth_keys (key, role, note) VALUES (?, ?, ?) RETURNING key, role, note, created_at;

-- name: UpdateAuthKey :one
UPDATE auth_keys
SET role = ?, note = ?
WHERE key = ?
RETURNING key, role, note, created_at;

-- name: DeleteAuthKey :execrows
DELETE FROM auth_keys WHERE key = ?;

-- name: CountAuthKeysByRole :one
SELECT COUNT(*) FROM auth_keys WHERE role = ?;
