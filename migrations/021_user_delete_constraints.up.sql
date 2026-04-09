-- band_invites: cascade delete when creator or acceptor is deleted
ALTER TABLE band_invites
    DROP CONSTRAINT band_invites_created_by_fkey,
    ADD CONSTRAINT band_invites_created_by_fkey
        FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE band_invites
    DROP CONSTRAINT band_invites_used_by_fkey,
    ADD CONSTRAINT band_invites_used_by_fkey
        FOREIGN KEY (used_by) REFERENCES users(id) ON DELETE SET NULL;

-- bands: set null so the band survives
ALTER TABLE bands ALTER COLUMN created_by DROP NOT NULL;
ALTER TABLE bands
    DROP CONSTRAINT bands_created_by_fkey,
    ADD CONSTRAINT bands_created_by_fkey
        FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;

-- tracks: set null so tracks survive
ALTER TABLE tracks ALTER COLUMN uploaded_by DROP NOT NULL;
ALTER TABLE tracks
    DROP CONSTRAINT tracks_uploaded_by_fkey,
    ADD CONSTRAINT tracks_uploaded_by_fkey
        FOREIGN KEY (uploaded_by) REFERENCES users(id) ON DELETE SET NULL;

-- comments: set null so thread history survives
ALTER TABLE comments ALTER COLUMN user_id DROP NOT NULL;
ALTER TABLE comments
    DROP CONSTRAINT comments_user_id_fkey,
    ADD CONSTRAINT comments_user_id_fkey
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL;

-- band_files: set null so files survive
ALTER TABLE band_files ALTER COLUMN uploaded_by DROP NOT NULL;
ALTER TABLE band_files
    DROP CONSTRAINT band_files_uploaded_by_fkey,
    ADD CONSTRAINT band_files_uploaded_by_fkey
        FOREIGN KEY (uploaded_by) REFERENCES users(id) ON DELETE SET NULL;
