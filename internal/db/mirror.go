package db

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Project mirrors (GarageBand .band packages synced file-by-file)

type MirrorProject struct {
	Name        string    `json:"name"`
	FileCount   int       `json:"file_count"`
	TotalSize   int64     `json:"total_size"`
	UpdatedAt   time.Time `json:"updated_at"`
	SourceLabel string    `json:"source_label"`
}

type MirrorFile struct {
	Path      string `json:"path"`
	Hash      string `json:"hash"`
	Size      int64  `json:"size"`
	ObjectKey string `json:"-"`
}

func (q *Queries) ListMirrorProjects(ctx context.Context, bandID uuid.UUID) ([]MirrorProject, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT p.name, COUNT(f.id), COALESCE(SUM(f.file_size), 0), p.updated_at, p.source_label
		FROM mirror_projects p
		LEFT JOIN mirror_files f ON f.project_id = p.id
		WHERE p.band_id = $1 AND p.deleted_at IS NULL
		GROUP BY p.id
		ORDER BY p.updated_at DESC
	`, bandID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MirrorProject
	for rows.Next() {
		var p MirrorProject
		if err := rows.Scan(&p.Name, &p.FileCount, &p.TotalSize, &p.UpdatedAt, &p.SourceLabel); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (q *Queries) ListMirrorFiles(ctx context.Context, bandID uuid.UUID, project string) ([]MirrorFile, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT f.rel_path, f.file_hash, f.file_size, f.object_key
		FROM mirror_files f
		JOIN mirror_projects p ON p.id = f.project_id
		WHERE p.band_id = $1 AND p.name = $2
		ORDER BY f.rel_path
	`, bandID, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MirrorFile
	for rows.Next() {
		var f MirrorFile
		if err := rows.Scan(&f.Path, &f.Hash, &f.Size, &f.ObjectKey); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (q *Queries) GetMirrorFile(ctx context.Context, bandID uuid.UUID, project, relPath string) (*MirrorFile, error) {
	var f MirrorFile
	err := q.pool.QueryRow(ctx, `
		SELECT f.rel_path, f.file_hash, f.file_size, f.object_key
		FROM mirror_files f
		JOIN mirror_projects p ON p.id = f.project_id
		WHERE p.band_id = $1 AND p.name = $2 AND f.rel_path = $3
	`, bandID, project, relPath).Scan(&f.Path, &f.Hash, &f.Size, &f.ObjectKey)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// UpsertMirrorFile records a file version, creating the project on first use.
// The object key is deterministic, so a changed file overwrites the same key
// and the bucket keeps the previous version.
func (q *Queries) UpsertMirrorFile(ctx context.Context, bandID, userID uuid.UUID, project string, f MirrorFile) error {
	tx, err := q.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// The WHERE makes a deleted project return no row, so a client that
	// doesn't know about deletion can't write files into it.
	var projectID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO mirror_projects (band_id, name, created_by)
		VALUES ($1, $2, $3)
		ON CONFLICT (band_id, name) DO UPDATE SET updated_at = now()
		WHERE mirror_projects.deleted_at IS NULL
		RETURNING id
	`, bandID, project, userID).Scan(&projectID)
	if err == pgx.ErrNoRows {
		return ErrMirrorProjectDeleted
	}
	if err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO mirror_files (project_id, rel_path, file_hash, file_size, object_key)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (project_id, rel_path) DO UPDATE SET
			file_hash = $3, file_size = $4, object_key = $5, updated_at = now()
	`, projectID, f.Path, f.Hash, f.Size, f.ObjectKey); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// DeleteMirrorFile removes a file row and returns its object key (empty if it didn't exist).
func (q *Queries) DeleteMirrorFile(ctx context.Context, bandID uuid.UUID, project, relPath string) (string, error) {
	var key string
	err := q.pool.QueryRow(ctx, `
		DELETE FROM mirror_files f
		USING mirror_projects p
		WHERE p.id = f.project_id AND p.band_id = $1 AND p.name = $2 AND f.rel_path = $3
		RETURNING f.object_key
	`, bandID, project, relPath).Scan(&key)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	if err == nil {
		_, err = q.pool.Exec(ctx, `
			UPDATE mirror_projects SET updated_at = now() WHERE band_id = $1 AND name = $2
		`, bandID, project)
	}
	return key, err
}

// MirrorSource identifies the copy of a project that owns the backup: an
// opaque id (install + absolute path) and a human-readable label for prompts.
type MirrorSource struct {
	ID    string `json:"source_id"`
	Label string `json:"source_label"`
}

// ErrMirrorProjectDeleted means the project was deleted and hasn't been
// deliberately backed up again since.
var ErrMirrorProjectDeleted = errors.New("mirror project was deleted")

// MirrorProjectState is what a client needs before pushing: who owns the
// backup, and whether it was deleted (and by whom).
type MirrorProjectState struct {
	MirrorSource
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	DeletedBy string     `json:"deleted_by,omitempty"`
}

// GetMirrorProjectState returns an empty state if the project doesn't exist.
func (q *Queries) GetMirrorProjectState(ctx context.Context, bandID uuid.UUID, project string) (MirrorProjectState, error) {
	var s MirrorProjectState
	err := q.pool.QueryRow(ctx, `
		SELECT p.source_id, p.source_label, p.deleted_at,
		       COALESCE(NULLIF(u.display_name, ''), u.email, '')
		FROM mirror_projects p
		LEFT JOIN users u ON u.id = p.deleted_by
		WHERE p.band_id = $1 AND p.name = $2
	`, bandID, project).Scan(&s.ID, &s.Label, &s.DeletedAt, &s.DeletedBy)
	if err == pgx.ErrNoRows {
		return MirrorProjectState{}, nil
	}
	if s.DeletedAt == nil {
		s.DeletedBy = ""
	}
	return s, err
}

// ClaimMirrorProject records which copy now owns the project, creating the
// project row if this is its first push.
func (q *Queries) ClaimMirrorProject(ctx context.Context, bandID, userID uuid.UUID, project string, src MirrorSource) error {
	_, err := q.pool.Exec(ctx, `
		INSERT INTO mirror_projects (band_id, name, created_by, source_id, source_label)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (band_id, name) DO UPDATE SET
			source_id = $4, source_label = $5, updated_at = now(),
			deleted_at = NULL, deleted_by = NULL
	`, bandID, project, userID, src.ID, src.Label)
	return err
}

// DeleteMirrorProject removes a project's file records and leaves the project
// row as a tombstone. Returns the number of files removed, and false if no
// such project exists. Running it again on a deleted project is harmless.
func (q *Queries) DeleteMirrorProject(ctx context.Context, bandID, userID uuid.UUID, project string) (int64, bool, error) {
	tx, err := q.pool.Begin(ctx)
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback(ctx)

	var projectID uuid.UUID
	err = tx.QueryRow(ctx, `
		UPDATE mirror_projects
		SET deleted_at = COALESCE(deleted_at, now()),
		    deleted_by = COALESCE(deleted_by, $3),
		    source_id = '', source_label = '', updated_at = now()
		WHERE band_id = $1 AND name = $2
		RETURNING id
	`, bandID, project, userID).Scan(&projectID)
	if err == pgx.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}

	tag, err := tx.Exec(ctx, `DELETE FROM mirror_files WHERE project_id = $1`, projectID)
	if err != nil {
		return 0, true, err
	}
	return tag.RowsAffected(), true, tx.Commit(ctx)
}
