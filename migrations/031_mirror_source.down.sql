ALTER TABLE mirror_projects
    DROP COLUMN IF EXISTS source_label,
    DROP COLUMN IF EXISTS source_id;
