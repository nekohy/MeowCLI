-- name: CountModels :one
SELECT COUNT(*) FROM models;

-- name: CountModelsByHandler :one
SELECT COUNT(*) FROM models WHERE handler = ?;

-- name: ReverseInfoFromModel :one
SELECT origin, handler, plan_types, plugin, content_affinity, fill_first, extra
FROM models
WHERE alias = ?
LIMIT 1;

-- name: ListModels :many
SELECT alias, origin, handler, plan_types, plugin, content_affinity, fill_first, extra
FROM models
ORDER BY alias;

-- name: CreateModel :one
INSERT INTO models (alias, origin, handler, plan_types, plugin, content_affinity, fill_first, extra)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING alias, origin, handler, plan_types, plugin, content_affinity, fill_first, extra;

-- name: UpdateModel :one
UPDATE models
SET origin = ?, handler = ?, plan_types = ?, plugin = ?, content_affinity = ?, fill_first = ?, extra = ?
WHERE alias = ?
RETURNING alias, origin, handler, plan_types, plugin, content_affinity, fill_first, extra;

-- name: DeleteModel :execrows
DELETE FROM models WHERE alias = ?;
