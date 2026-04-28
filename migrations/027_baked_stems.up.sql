ALTER TABLE tracks ADD COLUMN kind TEXT;

CREATE TABLE sync_baked_stems (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    track_id     UUID NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    reaper_guid  TEXT NOT NULL,
    reaper_name  TEXT NOT NULL,
    render_hash  TEXT NOT NULL,
    overdub_id   UUID REFERENCES tracks(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (track_id, reaper_guid)
);

CREATE INDEX idx_sync_baked_stems_track_id ON sync_baked_stems(track_id);
