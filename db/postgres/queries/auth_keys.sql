-- name: CountAuthKeys :one
SELECT COUNT(*) FROM auth_keys;

-- name: ListAuthKeys :many
SELECT key, role, note, created_at FROM auth_keys ORDER BY created_at;

-- name: GetAuthKey :one
SELECT key, role, note, created_at FROM auth_keys WHERE key = $1;

-- name: CreateAuthKey :one
INSERT INTO auth_keys (key, role, note) VALUES ($1, $2, $3) RETURNING key, role, note, created_at;

-- name: UpdateAuthKey :one
UPDATE auth_keys
SET role = $2, note = $3
WHERE key = $1
RETURNING key, role, note, created_at;

-- name: DeleteAuthKey :execrows
DELETE FROM auth_keys WHERE key = $1;

-- name: CountAuthKeysByRole :one
SELECT COUNT(*) FROM auth_keys WHERE role = $1;
