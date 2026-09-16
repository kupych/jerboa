-- File-level mirrors of DAW project packages we can't parse (GarageBand .band).
-- Bytes live in the mirror bucket (B2); these tables are the index.
CREATE TABLE mirror_projects (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    band_id     UUID NOT NULL REFERENCES bands(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    created_by  UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (band_id, name)
);

CREATE TABLE mirror_files (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id  UUID NOT NULL REFERENCES mirror_projects(id) ON DELETE CASCADE,
    rel_path    TEXT NOT NULL,
    file_hash   TEXT NOT NULL,
    file_size   BIGINT NOT NULL,
    object_key  TEXT NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, rel_path)
);
