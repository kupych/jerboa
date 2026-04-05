ALTER TABLE tracks ADD COLUMN rpp_session_name TEXT;

CREATE TABLE rpp_versions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    track_id    UUID NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    storage_key TEXT NOT NULL,
    version     INT NOT NULL DEFAULT 1,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE sync_files (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    track_id    UUID NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    overdub_id  UUID REFERENCES tracks(id) ON DELETE SET NULL,
    filename    TEXT NOT NULL,
    file_hash   TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (track_id, filename)
);

CREATE INDEX idx_rpp_versions_track_id ON rpp_versions(track_id);
CREATE INDEX idx_sync_files_track_id ON sync_files(track_id);
