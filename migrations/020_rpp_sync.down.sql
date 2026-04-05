DROP TABLE IF EXISTS sync_files;
DROP TABLE IF EXISTS rpp_versions;
ALTER TABLE tracks DROP COLUMN IF EXISTS rpp_session_name;
