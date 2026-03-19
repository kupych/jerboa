CREATE TABLE songs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    band_id     UUID NOT NULL REFERENCES bands(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_songs_band_id ON songs(band_id);

ALTER TABLE tracks ADD COLUMN song_id UUID REFERENCES songs(id) ON DELETE SET NULL;
ALTER TABLE tracks ADD COLUMN source_url TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_tracks_song_id ON tracks(song_id);
