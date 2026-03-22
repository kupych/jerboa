ALTER TABLE tracks ADD COLUMN overdub_of UUID REFERENCES tracks(id) ON DELETE SET NULL;
ALTER TABLE tracks ADD COLUMN offset_ms BIGINT NOT NULL DEFAULT 0;

CREATE INDEX idx_tracks_overdub_of ON tracks(overdub_of) WHERE overdub_of IS NOT NULL;

CREATE TABLE overdub_votes (
    track_id    UUID NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    overdub_id  UUID NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (track_id, user_id)
);
