ALTER TABLE band_invites
    DROP CONSTRAINT band_invites_created_by_fkey,
    ADD CONSTRAINT band_invites_created_by_fkey
        FOREIGN KEY (created_by) REFERENCES users(id);

ALTER TABLE band_invites
    DROP CONSTRAINT band_invites_used_by_fkey,
    ADD CONSTRAINT band_invites_used_by_fkey
        FOREIGN KEY (used_by) REFERENCES users(id);

ALTER TABLE bands
    DROP CONSTRAINT bands_created_by_fkey,
    ADD CONSTRAINT bands_created_by_fkey
        FOREIGN KEY (created_by) REFERENCES users(id);

ALTER TABLE tracks
    DROP CONSTRAINT tracks_uploaded_by_fkey,
    ADD CONSTRAINT tracks_uploaded_by_fkey
        FOREIGN KEY (uploaded_by) REFERENCES users(id);

ALTER TABLE comments
    DROP CONSTRAINT comments_user_id_fkey,
    ADD CONSTRAINT comments_user_id_fkey
        FOREIGN KEY (user_id) REFERENCES users(id);

ALTER TABLE band_files
    DROP CONSTRAINT band_files_uploaded_by_fkey,
    ADD CONSTRAINT band_files_uploaded_by_fkey
        FOREIGN KEY (uploaded_by) REFERENCES users(id);
