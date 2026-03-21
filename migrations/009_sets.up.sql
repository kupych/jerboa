CREATE TABLE sets (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    band_id     UUID NOT NULL REFERENCES bands(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    set_type    TEXT NOT NULL DEFAULT 'rehearsal'
                CHECK (set_type IN ('live', 'rehearsal', 'pre-production', 'other')),
    recorded_at DATE,
    notes       TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_sets_band_id ON sets(band_id);

CREATE TABLE set_items (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    set_id      UUID NOT NULL REFERENCES sets(id) ON DELETE CASCADE,
    position    INT NOT NULL,
    song_id     UUID REFERENCES songs(id) ON DELETE SET NULL,
    custom_name TEXT NOT NULL DEFAULT '',
    start_ms    BIGINT,
    end_ms      BIGINT,
    notes       TEXT NOT NULL DEFAULT '',
    UNIQUE(set_id, position)
);

CREATE INDEX idx_set_items_set_id ON set_items(set_id);

ALTER TABLE tracks ADD COLUMN recorded_at DATE;
ALTER TABLE tracks ADD COLUMN set_id UUID REFERENCES sets(id) ON DELETE SET NULL;
