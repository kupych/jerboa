-- Deleting a mirrored project leaves its row behind as a tombstone. Machines
-- that still remember the project would otherwise quietly re-upload it on
-- their next sync; the tombstone makes them ask first.
ALTER TABLE mirror_projects
    ADD COLUMN deleted_at TIMESTAMPTZ,
    ADD COLUMN deleted_by UUID REFERENCES users(id) ON DELETE SET NULL;
