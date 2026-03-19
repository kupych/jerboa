package db

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"jerboa/internal/models"
)

type Queries struct {
	pool *pgxpool.Pool
}

func NewQueries(pool *pgxpool.Pool) *Queries {
	return &Queries{pool: pool}
}

// Users

func (q *Queries) UpsertUser(ctx context.Context, email, displayName, avatarURL string) (*models.User, error) {
	var u models.User
	err := q.pool.QueryRow(ctx, `
		INSERT INTO users (email, display_name, avatar_url)
		VALUES ($1, $2, $3)
		ON CONFLICT (email) DO UPDATE SET
			display_name = COALESCE(NULLIF($2, ''), users.display_name),
			avatar_url = COALESCE(NULLIF($3, ''), users.avatar_url)
		RETURNING id, email, display_name, avatar_url, created_at
	`, email, displayName, avatarURL).Scan(&u.ID, &u.Email, &u.DisplayName, &u.AvatarURL, &u.CreatedAt)
	return &u, err
}

func (q *Queries) GetUser(ctx context.Context, id uuid.UUID) (*models.User, error) {
	var u models.User
	err := q.pool.QueryRow(ctx, `
		SELECT id, email, display_name, avatar_url, created_at
		FROM users WHERE id = $1
	`, id).Scan(&u.ID, &u.Email, &u.DisplayName, &u.AvatarURL, &u.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &u, err
}

// Sessions

func (q *Queries) CreateSession(ctx context.Context, userID uuid.UUID, ttl time.Duration) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	_, err := q.pool.Exec(ctx, `
		INSERT INTO sessions (token, user_id, expires_at)
		VALUES ($1, $2, $3)
	`, token, userID, time.Now().Add(ttl))
	return token, err
}

func (q *Queries) GetSession(ctx context.Context, token string) (*models.User, error) {
	var u models.User
	err := q.pool.QueryRow(ctx, `
		SELECT u.id, u.email, u.display_name, u.avatar_url, u.created_at
		FROM sessions s JOIN users u ON s.user_id = u.id
		WHERE s.token = $1 AND s.expires_at > now()
	`, token).Scan(&u.ID, &u.Email, &u.DisplayName, &u.AvatarURL, &u.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &u, err
}

func (q *Queries) DeleteSession(ctx context.Context, token string) error {
	_, err := q.pool.Exec(ctx, "DELETE FROM sessions WHERE token = $1", token)
	return err
}

// Bands

func (q *Queries) CreateBand(ctx context.Context, name, slug string, createdBy uuid.UUID) (*models.Band, error) {
	var b models.Band
	err := q.pool.QueryRow(ctx, `
		INSERT INTO bands (name, slug, created_by) VALUES ($1, $2, $3)
		RETURNING id, name, slug, created_by, created_at
	`, name, slug, createdBy).Scan(&b.ID, &b.Name, &b.Slug, &b.CreatedBy, &b.CreatedAt)
	if err != nil {
		return nil, err
	}
	_, err = q.pool.Exec(ctx, `
		INSERT INTO band_members (band_id, user_id, role) VALUES ($1, $2, 'admin')
	`, b.ID, createdBy)
	return &b, err
}

func (q *Queries) GetBandBySlug(ctx context.Context, slug string) (*models.Band, error) {
	var b models.Band
	err := q.pool.QueryRow(ctx, `
		SELECT id, name, slug, created_by, created_at FROM bands WHERE slug = $1
	`, slug).Scan(&b.ID, &b.Name, &b.Slug, &b.CreatedBy, &b.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &b, err
}

func (q *Queries) ListUserBands(ctx context.Context, userID uuid.UUID) ([]models.BandWithRole, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT b.id, b.name, b.slug, b.created_by, b.created_at, bm.role
		FROM bands b JOIN band_members bm ON b.id = bm.band_id
		WHERE bm.user_id = $1
		ORDER BY b.name
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bands []models.BandWithRole
	for rows.Next() {
		var b models.BandWithRole
		if err := rows.Scan(&b.ID, &b.Name, &b.Slug, &b.CreatedBy, &b.CreatedAt, &b.Role); err != nil {
			return nil, err
		}
		bands = append(bands, b)
	}
	return bands, nil
}

func (q *Queries) IsBandMember(ctx context.Context, bandID, userID uuid.UUID) (bool, string, error) {
	var role string
	err := q.pool.QueryRow(ctx, `
		SELECT role FROM band_members WHERE band_id = $1 AND user_id = $2
	`, bandID, userID).Scan(&role)
	if err == pgx.ErrNoRows {
		return false, "", nil
	}
	return err == nil, role, err
}

func (q *Queries) GetBandMembers(ctx context.Context, bandID uuid.UUID) ([]models.BandMember, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT bm.band_id, bm.user_id, bm.role, bm.joined_at,
		       u.id, u.email, u.display_name, u.avatar_url, u.created_at
		FROM band_members bm JOIN users u ON bm.user_id = u.id
		WHERE bm.band_id = $1
		ORDER BY bm.joined_at
	`, bandID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []models.BandMember
	for rows.Next() {
		var m models.BandMember
		var u models.User
		if err := rows.Scan(&m.BandID, &m.UserID, &m.Role, &m.JoinedAt,
			&u.ID, &u.Email, &u.DisplayName, &u.AvatarURL, &u.CreatedAt); err != nil {
			return nil, err
		}
		m.User = &u
		members = append(members, m)
	}
	return members, nil
}

func (q *Queries) CreateInvite(ctx context.Context, bandID, createdBy uuid.UUID, email string, ttl time.Duration) (*models.BandInvite, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(b)

	var inv models.BandInvite
	err := q.pool.QueryRow(ctx, `
		INSERT INTO band_invites (band_id, token, created_by, email, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, band_id, token, created_by, expires_at, created_at
	`, bandID, token, createdBy, email, time.Now().Add(ttl)).Scan(
		&inv.ID, &inv.BandID, &inv.Token, &inv.CreatedBy, &inv.ExpiresAt, &inv.CreatedAt)
	return &inv, err
}

func (q *Queries) HasPendingInvite(ctx context.Context, email string) (bool, error) {
	var exists bool
	err := q.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM band_invites
			WHERE email = $1 AND expires_at > now() AND used_by IS NULL
		)
	`, email).Scan(&exists)
	return exists, err
}

func (q *Queries) UserExists(ctx context.Context, email string) (bool, error) {
	var exists bool
	err := q.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)
	`, email).Scan(&exists)
	return exists, err
}

func (q *Queries) AcceptInvite(ctx context.Context, token string, userID uuid.UUID) (*models.Band, error) {
	var bandID uuid.UUID
	err := q.pool.QueryRow(ctx, `
		UPDATE band_invites SET used_by = $1, used_at = now()
		WHERE token = $2 AND expires_at > now() AND used_by IS NULL
		RETURNING band_id
	`, userID, token).Scan(&bandID)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	_, err = q.pool.Exec(ctx, `
		INSERT INTO band_members (band_id, user_id, role)
		VALUES ($1, $2, 'member')
		ON CONFLICT DO NOTHING
	`, bandID, userID)
	if err != nil {
		return nil, err
	}

	return q.GetBandBySlug(ctx, "")
}

func (q *Queries) AcceptInviteReturningBand(ctx context.Context, token string, userID uuid.UUID) (*models.Band, error) {
	tx, err := q.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var bandID uuid.UUID
	err = tx.QueryRow(ctx, `
		UPDATE band_invites SET used_by = $1, used_at = now()
		WHERE token = $2 AND expires_at > now() AND used_by IS NULL
		RETURNING band_id
	`, userID, token).Scan(&bandID)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO band_members (band_id, user_id, role)
		VALUES ($1, $2, 'member')
		ON CONFLICT DO NOTHING
	`, bandID, userID)
	if err != nil {
		return nil, err
	}

	var b models.Band
	err = tx.QueryRow(ctx, `
		SELECT id, name, slug, created_by, created_at FROM bands WHERE id = $1
	`, bandID).Scan(&b.ID, &b.Name, &b.Slug, &b.CreatedBy, &b.CreatedAt)
	if err != nil {
		return nil, err
	}

	return &b, tx.Commit(ctx)
}

// Songs

func (q *Queries) CreateSong(ctx context.Context, bandID uuid.UUID, name string) (*models.Song, error) {
	var s models.Song
	err := q.pool.QueryRow(ctx, `
		INSERT INTO songs (band_id, name) VALUES ($1, $2)
		RETURNING id, band_id, name, created_at
	`, bandID, name).Scan(&s.ID, &s.BandID, &s.Name, &s.CreatedAt)
	return &s, err
}

func (q *Queries) ListSongs(ctx context.Context, bandID uuid.UUID) ([]models.Song, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT id, band_id, name, created_at FROM songs
		WHERE band_id = $1 ORDER BY name
	`, bandID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var songs []models.Song
	for rows.Next() {
		var s models.Song
		if err := rows.Scan(&s.ID, &s.BandID, &s.Name, &s.CreatedAt); err != nil {
			return nil, err
		}
		songs = append(songs, s)
	}
	return songs, nil
}

func (q *Queries) UpdateSong(ctx context.Context, id uuid.UUID, name string) error {
	_, err := q.pool.Exec(ctx, `UPDATE songs SET name = $2 WHERE id = $1`, id, name)
	return err
}

func (q *Queries) DeleteSong(ctx context.Context, id uuid.UUID) error {
	_, err := q.pool.Exec(ctx, `DELETE FROM songs WHERE id = $1`, id)
	return err
}

func (q *Queries) SetTrackSong(ctx context.Context, trackID uuid.UUID, songID *uuid.UUID) error {
	_, err := q.pool.Exec(ctx, `UPDATE tracks SET song_id = $2 WHERE id = $1`, trackID, songID)
	return err
}

// Tracks

func (q *Queries) CreateTrack(ctx context.Context, t *models.Track) error {
	return q.pool.QueryRow(ctx, `
		INSERT INTO tracks (band_id, title, description, uploaded_by, file_path, file_size, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at
	`, t.BandID, t.Title, t.Description, t.UploadedBy, t.FilePath, t.FileSize, t.Status).Scan(&t.ID, &t.CreatedAt)
}

func (q *Queries) UpdateTrackFile(ctx context.Context, id uuid.UUID, filePath string, fileSize int64) error {
	_, err := q.pool.Exec(ctx, `UPDATE tracks SET file_path = $2, file_size = $3 WHERE id = $1`, id, filePath, fileSize)
	return err
}

func (q *Queries) UpdateTrackProcessed(ctx context.Context, id uuid.UUID, waveform []byte, durationMS int64, format string, sampleRate int) error {
	_, err := q.pool.Exec(ctx, `
		UPDATE tracks SET waveform_data = $2, duration_ms = $3, format = $4,
		       sample_rate = $5, status = 'ready'
		WHERE id = $1
	`, id, waveform, durationMS, format, sampleRate)
	return err
}

func (q *Queries) UpdateTrackError(ctx context.Context, id uuid.UUID) error {
	_, err := q.pool.Exec(ctx, "UPDATE tracks SET status = 'error' WHERE id = $1", id)
	return err
}

func (q *Queries) GetTrack(ctx context.Context, id uuid.UUID) (*models.Track, error) {
	var t models.Track
	var u models.User
	var songID *uuid.UUID
	var songName *string
	err := q.pool.QueryRow(ctx, `
		SELECT t.id, t.band_id, t.title, t.description, t.uploaded_by, t.file_path,
		       t.waveform_data, t.duration_ms, t.format, t.sample_rate, t.file_size,
		       t.status, t.tags, t.notes, t.song_id, t.source_url, t.created_at,
		       u.id, u.email, u.display_name, u.avatar_url, u.created_at,
		       s.name
		FROM tracks t
		JOIN users u ON t.uploaded_by = u.id
		LEFT JOIN songs s ON t.song_id = s.id
		WHERE t.id = $1
	`, id).Scan(&t.ID, &t.BandID, &t.Title, &t.Description, &t.UploadedBy, &t.FilePath,
		&t.WaveformData, &t.DurationMS, &t.Format, &t.SampleRate, &t.FileSize,
		&t.Status, &t.Tags, &t.Notes, &songID, &t.SourceURL, &t.CreatedAt,
		&u.ID, &u.Email, &u.DisplayName, &u.AvatarURL, &u.CreatedAt,
		&songName)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	t.Uploader = &u
	t.SongID = songID
	if songID != nil && songName != nil {
		t.Song = &models.Song{ID: *songID, Name: *songName}
	}
	return &t, err
}

func (q *Queries) ListTracks(ctx context.Context, bandID uuid.UUID) ([]models.Track, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT t.id, t.band_id, t.title, t.description, t.uploaded_by, t.file_path,
		       t.waveform_data, t.duration_ms, t.format, t.sample_rate, t.file_size,
		       t.status, t.tags, t.notes, t.song_id, t.source_url, t.created_at,
		       u.id, u.email, u.display_name, u.avatar_url, u.created_at,
		       s.name
		FROM tracks t
		JOIN users u ON t.uploaded_by = u.id
		LEFT JOIN songs s ON t.song_id = s.id
		WHERE t.band_id = $1
		ORDER BY t.created_at DESC
	`, bandID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tracks []models.Track
	for rows.Next() {
		var t models.Track
		var u models.User
		var songID *uuid.UUID
		var songName *string
		if err := rows.Scan(&t.ID, &t.BandID, &t.Title, &t.Description, &t.UploadedBy, &t.FilePath,
			&t.WaveformData, &t.DurationMS, &t.Format, &t.SampleRate, &t.FileSize,
			&t.Status, &t.Tags, &t.Notes, &songID, &t.SourceURL, &t.CreatedAt,
			&u.ID, &u.Email, &u.DisplayName, &u.AvatarURL, &u.CreatedAt,
			&songName); err != nil {
			return nil, err
		}
		t.Uploader = &u
		t.SongID = songID
		if songID != nil && songName != nil {
			t.Song = &models.Song{ID: *songID, Name: *songName}
		}
		tracks = append(tracks, t)
	}
	return tracks, nil
}

func (q *Queries) UpdateTrackMeta(ctx context.Context, id uuid.UUID, title, description, notes string) error {
	_, err := q.pool.Exec(ctx, `UPDATE tracks SET title = $2, description = $3, notes = $4 WHERE id = $1`, id, title, description, notes)
	return err
}

func (q *Queries) UpdateTrackTags(ctx context.Context, id uuid.UUID, tags []string) error {
	_, err := q.pool.Exec(ctx, `UPDATE tracks SET tags = $2 WHERE id = $1`, id, tags)
	return err
}

func (q *Queries) DeleteTrack(ctx context.Context, id uuid.UUID) (string, error) {
	var filePath string
	err := q.pool.QueryRow(ctx, "DELETE FROM tracks WHERE id = $1 RETURNING file_path", id).Scan(&filePath)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return filePath, err
}

// Track Personnel

func (q *Queries) ListTrackPersonnel(ctx context.Context, trackID uuid.UUID) ([]models.TrackPersonnel, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT tp.track_id, tp.user_id, tp.role,
		       u.id, u.email, u.display_name, u.avatar_url, u.created_at
		FROM track_personnel tp JOIN users u ON tp.user_id = u.id
		WHERE tp.track_id = $1
		ORDER BY tp.role, u.display_name
	`, trackID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var personnel []models.TrackPersonnel
	for rows.Next() {
		var p models.TrackPersonnel
		var u models.User
		if err := rows.Scan(&p.TrackID, &p.UserID, &p.Role,
			&u.ID, &u.Email, &u.DisplayName, &u.AvatarURL, &u.CreatedAt); err != nil {
			return nil, err
		}
		p.User = &u
		personnel = append(personnel, p)
	}
	return personnel, nil
}

func (q *Queries) AddTrackPersonnel(ctx context.Context, trackID, userID uuid.UUID, role string) error {
	_, err := q.pool.Exec(ctx, `
		INSERT INTO track_personnel (track_id, user_id, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (track_id, user_id) DO UPDATE SET role = $3
	`, trackID, userID, role)
	return err
}

func (q *Queries) RemoveTrackPersonnel(ctx context.Context, trackID, userID uuid.UUID) error {
	_, err := q.pool.Exec(ctx, `DELETE FROM track_personnel WHERE track_id = $1 AND user_id = $2`, trackID, userID)
	return err
}

// Comments

func (q *Queries) CreateComment(ctx context.Context, c *models.Comment) error {
	return q.pool.QueryRow(ctx, `
		INSERT INTO comments (track_id, user_id, parent_id, body, timestamp_ms)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at
	`, c.TrackID, c.UserID, c.ParentID, c.Body, c.TimestampMS).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
}

func (q *Queries) ListComments(ctx context.Context, trackID uuid.UUID) ([]models.Comment, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT c.id, c.track_id, c.user_id, c.parent_id, c.body, c.timestamp_ms,
		       c.created_at, c.updated_at,
		       u.id, u.email, u.display_name, u.avatar_url, u.created_at
		FROM comments c JOIN users u ON c.user_id = u.id
		WHERE c.track_id = $1
		ORDER BY c.timestamp_ms ASC NULLS LAST, c.created_at ASC
	`, trackID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var all []models.Comment
	for rows.Next() {
		var c models.Comment
		var u models.User
		if err := rows.Scan(&c.ID, &c.TrackID, &c.UserID, &c.ParentID, &c.Body, &c.TimestampMS,
			&c.CreatedAt, &c.UpdatedAt,
			&u.ID, &u.Email, &u.DisplayName, &u.AvatarURL, &u.CreatedAt); err != nil {
			return nil, err
		}
		c.User = &u
		all = append(all, c)
	}

	// Build tree: top-level comments with nested replies
	byID := make(map[uuid.UUID]*models.Comment)
	var roots []models.Comment
	for i := range all {
		byID[all[i].ID] = &all[i]
	}
	for i := range all {
		if all[i].ParentID == nil {
			roots = append(roots, all[i])
		} else if parent, ok := byID[*all[i].ParentID]; ok {
			parent.Replies = append(parent.Replies, all[i])
		}
	}
	return roots, nil
}

func (q *Queries) UpdateComment(ctx context.Context, id, userID uuid.UUID, body string) error {
	tag, err := q.pool.Exec(ctx, `
		UPDATE comments SET body = $1, updated_at = now()
		WHERE id = $2 AND user_id = $3
	`, body, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (q *Queries) DeleteComment(ctx context.Context, id, userID uuid.UUID) error {
	tag, err := q.pool.Exec(ctx, "DELETE FROM comments WHERE id = $1 AND user_id = $2", id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// Chat

func (q *Queries) CreateChatMessage(ctx context.Context, bandID, userID uuid.UUID, body string) (*models.ChatMessage, error) {
	var m models.ChatMessage
	m.User = &models.User{}
	err := q.pool.QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO chat_messages (band_id, user_id, body)
			VALUES ($1, $2, $3)
			RETURNING id, band_id, user_id, body, created_at
		)
		SELECT i.id, i.band_id, i.user_id, i.body, i.created_at,
		       u.display_name, u.email, u.avatar_url
		FROM inserted i
		JOIN users u ON u.id = i.user_id
	`, bandID, userID, body).Scan(
		&m.ID, &m.BandID, &m.UserID, &m.Body, &m.CreatedAt,
		&m.User.DisplayName, &m.User.Email, &m.User.AvatarURL,
	)
	if err != nil {
		return nil, err
	}
	m.User.ID = m.UserID
	return &m, nil
}

func (q *Queries) ListChatMessages(ctx context.Context, bandID uuid.UUID, limit int) ([]models.ChatMessage, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT m.id, m.band_id, m.user_id, m.body, m.created_at,
		       u.display_name, u.email, u.avatar_url
		FROM chat_messages m
		JOIN users u ON u.id = m.user_id
		WHERE m.band_id = $1
		ORDER BY m.created_at DESC
		LIMIT $2
	`, bandID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []models.ChatMessage
	for rows.Next() {
		var m models.ChatMessage
		m.User = &models.User{}
		if err := rows.Scan(
			&m.ID, &m.BandID, &m.UserID, &m.Body, &m.CreatedAt,
			&m.User.DisplayName, &m.User.Email, &m.User.AvatarURL,
		); err != nil {
			return nil, err
		}
		m.User.ID = m.UserID
		messages = append(messages, m)
	}
	return messages, nil
}
