-- name: UpsertAntigravityCredits :one
INSERT INTO antigravity_credits (credential_id, credits_amount, credit_types, synced_at)
VALUES ($1, $2, $3, NOW())
ON CONFLICT (credential_id) DO UPDATE
SET
    credits_amount = excluded.credits_amount,
    credit_types = excluded.credit_types,
    synced_at = NOW()
RETURNING *;

-- name: GetAntigravityCredits :one
SELECT * FROM antigravity_credits WHERE credential_id = $1 LIMIT 1;
