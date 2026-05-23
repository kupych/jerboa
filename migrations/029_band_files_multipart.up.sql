ALTER TABLE band_files
  ADD COLUMN upload_id text NOT NULL DEFAULT '',
  ADD COLUMN status text NOT NULL DEFAULT 'complete';
