-- name: CountModels :one
SELECT COUNT(*) FROM models;

-- name: CountModelsByHandler :one
SELECT COUNT(*) FROM models WHERE handler = $1;

-- name: ReverseInfoFromModel :one
SELECT origin, handler, plan_types, extra
FROM models
WHERE alias = $1
LIMIT 1;

-- name: ListModels :many
SELECT * FROM models ORDER BY alias;

-- name: CreateModel :one
INSERT INTO models (alias, origin, handler, plan_types, extra)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateModel :one
UPDATE models
SET origin = $2, handler = $3, plan_types = $4, extra = $5
WHERE alias = $1
RETURNING *;

-- name: DeleteModel :execrows
DELETE FROM models WHERE alias = $1;
