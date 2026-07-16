-- name: CountModels :one
SELECT COUNT(*) FROM models;

-- name: CountModelsByHandler :one
SELECT COUNT(*) FROM models WHERE handler = $1;

-- name: ReverseInfoFromModel :one
SELECT origin, handler, plan_types, plugin, content_affinity, fill_first, extra
FROM models
WHERE alias = $1
LIMIT 1;

-- name: ListModels :many
SELECT alias, origin, handler, plan_types, plugin, content_affinity, fill_first, extra
FROM models
ORDER BY alias;

-- name: CreateModel :one
INSERT INTO models (alias, origin, handler, plan_types, plugin, content_affinity, fill_first, extra)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING alias, origin, handler, plan_types, plugin, content_affinity, fill_first, extra;

-- name: UpdateModel :one
UPDATE models
SET origin = $2, handler = $3, plan_types = $4, plugin = $5, content_affinity = $6, fill_first = $7, extra = $8
WHERE alias = $1
RETURNING alias, origin, handler, plan_types, plugin, content_affinity, fill_first, extra;

-- name: DeleteModel :execrows
DELETE FROM models WHERE alias = $1;
