-- Add reason column to codex table for soft-delete disable reasons.
ALTER TABLE codex ADD COLUMN IF NOT EXISTS reason TEXT NOT NULL DEFAULT '';