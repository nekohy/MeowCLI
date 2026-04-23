-- name: CountModels :one
SELECT COUNT(*) FROM models;

-- name: CountModelsByHandler :one
SELECT COUNT(*) FROM models WHERE handler = ?;

-- name: ReverseInfoFromModel :one
SELECT origin, handler, plan_types, extra
FROM models
WHERE alias = ?
LIMIT 1;

-- name: ListModels :many
SELECT * FROM models ORDER BY alias;

-- name: CreateModel :one
INSERT INTO models (alias, origin, handler, plan_types, extra)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateModel :one
UPDATE models
SET origin = ?, handler = ?, plan_types = ?, extra = ?
WHERE alias = ?
RETURNING *;

-- name: DeleteModel :execrows
DELETE FROM models WHERE alias = ?;
