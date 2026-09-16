-- Where a mirrored project was last pushed from. The server identifies a
-- project by name alone, so two different "My Song.band" folders would
-- otherwise overwrite each other's backup; clients compare this before
-- pushing and ask before replacing a copy from somewhere else.
ALTER TABLE mirror_projects
    ADD COLUMN source_id    TEXT NOT NULL DEFAULT '',
    ADD COLUMN source_label TEXT NOT NULL DEFAULT '';
