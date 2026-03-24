-- name: CountModels :one
SELECT COUNT(*) FROM models;

-- name: CountModelsByHandler :one
SELECT COUNT(*) FROM models WHERE handler = $1;

-- name: ReverseInfoFromModel :one
SELECT origin, handler, extra
FROM models
WHERE alias = $1
LIMIT 1;

-- name: ListModels :many
SELECT * FROM models ORDER BY alias;

-- name: CreateModel :one
INSERT INTO models (alias, origin, handler, extra)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdateModel :one
UPDATE models
SET origin = $2, handler = $3, extra = $4
WHERE alias = $1
RETURNING *;

-- name: DeleteModel :execrows
DELETE FROM models WHERE alias = $1;
