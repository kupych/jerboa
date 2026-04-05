CREATE TABLE band_files (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    band_id      UUID NOT NULL REFERENCES bands(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    storage_key  TEXT NOT NULL,
    file_size    BIGINT NOT NULL DEFAULT 0,
    content_type TEXT NOT NULL DEFAULT '',
    uploaded_by  UUID NOT NULL REFERENCES users(id),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_band_files_band_id ON band_files(band_id);
