package db

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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
		RETURNING id, email, display_name, avatar_url, is_admin, is_demo, created_at
	`, email, displayName, avatarURL).Scan(&u.ID, &u.Email, &u.DisplayName, &u.AvatarURL, &u.IsAdmin, &u.IsDemo, &u.CreatedAt)
	return &u, err
}

func (q *Queries) UpdateUserProfile(ctx context.Context, id uuid.UUID, displayName string) (*models.User, error) {
	var u models.User
	err := q.pool.QueryRow(ctx, `
		UPDATE users SET display_name = $2
		WHERE id = $1
		RETURNING id, email, display_name, avatar_url, is_admin, is_demo, created_at
	`, id, displayName).Scan(&u.ID, &u.Email, &u.DisplayName, &u.AvatarURL, &u.IsAdmin, &u.IsDemo, &u.CreatedAt)
	return &u, err
}

func (q *Queries) GetUser(ctx context.Context, id uuid.UUID) (*models.User, error) {
	var u models.User
	err := q.pool.QueryRow(ctx, `
		SELECT id, email, display_name, avatar_url, is_admin, is_demo, created_at
		FROM users WHERE id = $1
	`, id).Scan(&u.ID, &u.Email, &u.DisplayName, &u.AvatarURL, &u.IsAdmin, &u.IsDemo, &u.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &u, err
}

func (q *Queries) DeleteUser(ctx context.Context, id uuid.UUID) error {
	_, err := q.pool.Exec(ctx, "DELETE FROM users WHERE id = $1", id)
	return err
}

func (q *Queries) DeleteInvite(ctx context.Context, id uuid.UUID) error {
	_, err := q.pool.Exec(ctx, "DELETE FROM band_invites WHERE id = $1", id)
	return err
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
		SELECT u.id, u.email, u.display_name, u.avatar_url, u.is_admin, u.is_demo, u.created_at
		FROM sessions s JOIN users u ON s.user_id = u.id
		WHERE s.token = $1 AND s.expires_at > now()
	`, token).Scan(&u.ID, &u.Email, &u.DisplayName, &u.AvatarURL, &u.IsAdmin, &u.IsDemo, &u.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &u, err
}

func (q *Queries) DeleteSession(ctx context.Context, token string) error {
	_, err := q.pool.Exec(ctx, "DELETE FROM sessions WHERE token = $1", token)
	return err
}

// Magic links

func (q *Queries) CreateMagicLink(ctx context.Context, email string, ttl time.Duration) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	_, err := q.pool.Exec(ctx, `
		INSERT INTO magic_links (email, token, expires_at)
		VALUES ($1, $2, $3)
	`, email, token, time.Now().Add(ttl))
	return token, err
}

func (q *Queries) GetMagicLink(ctx context.Context, token string) (string, error) {
	var email string
	err := q.pool.QueryRow(ctx, `
		SELECT email FROM magic_links
		WHERE token = $1 AND expires_at > now() AND used_at IS NULL
	`, token).Scan(&email)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return email, err
}

func (q *Queries) MarkMagicLinkUsed(ctx context.Context, token string) error {
	_, err := q.pool.Exec(ctx, `
		UPDATE magic_links SET used_at = now() WHERE token = $1
	`, token)
	return err
}

// Bands

func (q *Queries) CreateBand(ctx context.Context, name, slug string, createdBy uuid.UUID) (*models.Band, error) {
	var b models.Band
	err := q.pool.QueryRow(ctx, `
		INSERT INTO bands (name, slug, created_by) VALUES ($1, $2, $3)
		RETURNING id, name, slug, created_by, color_scheme, created_at
	`, name, slug, createdBy).Scan(&b.ID, &b.Name, &b.Slug, &b.CreatedBy, &b.ColorScheme, &b.CreatedAt)
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
		SELECT id, name, slug, created_by, color_scheme, created_at FROM bands WHERE slug = $1
	`, slug).Scan(&b.ID, &b.Name, &b.Slug, &b.CreatedBy, &b.ColorScheme, &b.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &b, err
}

func (q *Queries) ListAllBandsWithRole(ctx context.Context) ([]models.BandWithRole, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT b.id, b.name, b.slug, b.created_by, b.color_scheme, b.created_at, 'admin'
		FROM bands b
		ORDER BY b.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bands []models.BandWithRole
	for rows.Next() {
		var b models.BandWithRole
		if err := rows.Scan(&b.ID, &b.Name, &b.Slug, &b.CreatedBy, &b.ColorScheme, &b.CreatedAt, &b.Role); err != nil {
			return nil, err
		}
		bands = append(bands, b)
	}
	return bands, nil
}

func (q *Queries) ListUserBands(ctx context.Context, userID uuid.UUID) ([]models.BandWithRole, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT b.id, b.name, b.slug, b.created_by, b.color_scheme, b.created_at, bm.role
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
		if err := rows.Scan(&b.ID, &b.Name, &b.Slug, &b.CreatedBy, &b.ColorScheme, &b.CreatedAt, &b.Role); err != nil {
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
		       u.id, u.email, u.display_name, u.avatar_url, u.is_admin, u.is_demo, u.created_at
		FROM band_members bm JOIN users u ON bm.user_id = u.id
		WHERE bm.band_id = $1
		ORDER BY split_part(u.display_name, ' ', array_length(string_to_array(u.display_name, ' '), 1)), u.display_name
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
			&u.ID, &u.Email, &u.DisplayName, &u.AvatarURL, &u.IsAdmin, &u.IsDemo, &u.CreatedAt); err != nil {
			return nil, err
		}
		m.User = &u
		members = append(members, m)
	}
	return members, nil
}

func (q *Queries) RemoveBandMember(ctx context.Context, bandID, userID uuid.UUID) error {
	_, err := q.pool.Exec(ctx, `
		DELETE FROM band_members WHERE band_id = $1 AND user_id = $2
	`, bandID, userID)
	return err
}

func (q *Queries) UpdateBandMemberRole(ctx context.Context, bandID, userID uuid.UUID, role string) error {
	_, err := q.pool.Exec(ctx, `
		UPDATE band_members SET role = $3 WHERE band_id = $1 AND user_id = $2
	`, bandID, userID, role)
	return err
}

func (q *Queries) UpdateBandName(ctx context.Context, bandID uuid.UUID, name, slug string) (*models.Band, error) {
	var b models.Band
	err := q.pool.QueryRow(ctx, `
		UPDATE bands SET name = $2, slug = $3 WHERE id = $1
		RETURNING id, name, slug, created_by, color_scheme, created_at
	`, bandID, name, slug).Scan(&b.ID, &b.Name, &b.Slug, &b.CreatedBy, &b.ColorScheme, &b.CreatedAt)
	return &b, err
}

func (q *Queries) UpdateBandColorScheme(ctx context.Context, bandID uuid.UUID, colorScheme string) error {
	_, err := q.pool.Exec(ctx, `UPDATE bands SET color_scheme = $2 WHERE id = $1`, bandID, colorScheme)
	return err
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

func (q *Queries) GetInviteByToken(ctx context.Context, token string) (*models.BandInvite, string, error) {
	var inv models.BandInvite
	var email string
	err := q.pool.QueryRow(ctx, `
		SELECT i.id, i.band_id, i.token, i.created_by, i.expires_at, i.used_by, i.used_at, i.created_at, i.email
		FROM band_invites i
		WHERE i.token = $1
	`, token).Scan(&inv.ID, &inv.BandID, &inv.Token, &inv.CreatedBy, &inv.ExpiresAt, &inv.UsedBy, &inv.UsedAt, &inv.CreatedAt, &email)
	if err == pgx.ErrNoRows {
		return nil, "", nil
	}
	return &inv, email, err
}

func (q *Queries) EnsureBandMember(ctx context.Context, bandID, userID uuid.UUID) error {
	_, err := q.pool.Exec(ctx, `
		INSERT INTO band_members (band_id, user_id, role)
		VALUES ($1, $2, 'member')
		ON CONFLICT DO NOTHING
	`, bandID, userID)
	return err
}

func (q *Queries) MarkInviteUsed(ctx context.Context, token string, userID uuid.UUID) error {
	_, err := q.pool.Exec(ctx, `
		UPDATE band_invites SET used_by = $1, used_at = now()
		WHERE token = $2 AND used_by IS NULL
	`, userID, token)
	return err
}

func (q *Queries) GetPendingBandInvites(ctx context.Context, bandID uuid.UUID) ([]models.PendingInvite, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT i.id, i.email, i.created_at
		FROM band_invites i
		WHERE i.band_id = $1 AND i.used_by IS NULL AND i.expires_at > now()
		ORDER BY i.created_at DESC
	`, bandID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var invites []models.PendingInvite
	for rows.Next() {
		var inv models.PendingInvite
		if err := rows.Scan(&inv.ID, &inv.Email, &inv.CreatedAt); err != nil {
			return nil, err
		}
		invites = append(invites, inv)
	}
	return invites, nil
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

func (q *Queries) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	var u models.User
	err := q.pool.QueryRow(ctx, `
		SELECT id, email, display_name, avatar_url, is_admin, is_demo, created_at FROM users WHERE email = $1
	`, email).Scan(&u.ID, &u.Email, &u.DisplayName, &u.AvatarURL, &u.IsAdmin, &u.IsDemo, &u.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &u, err
}

func (q *Queries) GetDemoUser(ctx context.Context) (*models.User, error) {
	var u models.User
	err := q.pool.QueryRow(ctx, `
		SELECT id, email, display_name, avatar_url, is_admin, is_demo, created_at
		FROM users WHERE is_demo = true ORDER BY created_at LIMIT 1
	`).Scan(&u.ID, &u.Email, &u.DisplayName, &u.AvatarURL, &u.IsAdmin, &u.IsDemo, &u.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &u, err
}

func (q *Queries) AddBandMember(ctx context.Context, bandID, userID uuid.UUID, role string) error {
	_, err := q.pool.Exec(ctx, `
		INSERT INTO band_members (band_id, user_id, role)
		VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING
	`, bandID, userID, role)
	return err
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
		SELECT id, name, slug, created_by, color_scheme, created_at FROM bands WHERE id = $1
	`, bandID).Scan(&b.ID, &b.Name, &b.Slug, &b.CreatedBy, &b.ColorScheme, &b.CreatedAt)
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
		RETURNING id, band_id, name, lyrics, tabs, notes, bpm, created_at
	`, bandID, name).Scan(&s.ID, &s.BandID, &s.Name, &s.Lyrics, &s.Tabs, &s.Notes, &s.BPM, &s.CreatedAt)
	return &s, err
}

func (q *Queries) ListSongs(ctx context.Context, bandID uuid.UUID) ([]models.Song, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT s.id, s.band_id, s.name, s.lyrics, s.tabs, s.notes, s.bpm, s.created_at,
			COUNT(DISTINCT t.id) +
			COUNT(DISTINCT CASE WHEN st.id IS NOT NULL THEN si.set_id END) AS take_count
		FROM songs s
		LEFT JOIN tracks t ON t.song_id = s.id
		LEFT JOIN set_items si ON si.song_id = s.id
		LEFT JOIN sets st ON st.id = si.set_id
			AND EXISTS (SELECT 1 FROM tracks WHERE set_id = st.id)
		WHERE s.band_id = $1
		GROUP BY s.id
		ORDER BY s.name
	`, bandID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var songs []models.Song
	for rows.Next() {
		var s models.Song
		if err := rows.Scan(&s.ID, &s.BandID, &s.Name, &s.Lyrics, &s.Tabs, &s.Notes, &s.BPM, &s.CreatedAt, &s.TakeCount); err != nil {
			return nil, err
		}
		songs = append(songs, s)
	}
	return songs, nil
}

func (q *Queries) UpdateSong(ctx context.Context, id uuid.UUID, name, lyrics, tabs, notes string, bpm int) error {
	_, err := q.pool.Exec(ctx, `UPDATE songs SET name = $2, lyrics = $3, tabs = $4, notes = $5, bpm = $6 WHERE id = $1`, id, name, lyrics, tabs, notes, bpm)
	return err
}

func (q *Queries) GetSong(ctx context.Context, id uuid.UUID) (*models.Song, error) {
	var s models.Song
	err := q.pool.QueryRow(ctx, `
		SELECT id, band_id, name, lyrics, tabs, notes, bpm, created_at FROM songs WHERE id = $1
	`, id).Scan(&s.ID, &s.BandID, &s.Name, &s.Lyrics, &s.Tabs, &s.Notes, &s.BPM, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
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
	gain := t.Gain
	if gain == 0 {
		gain = 1.0
	}
	return q.pool.QueryRow(ctx, `
		INSERT INTO tracks (band_id, title, description, uploaded_by, file_path, file_size, status, overdub_of, offset_ms, bounced_to, gain)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, created_at
	`, t.BandID, t.Title, t.Description, t.UploadedBy, t.FilePath, t.FileSize, t.Status, t.OverdubOf, t.OffsetMS, t.BouncedTo, gain).Scan(&t.ID, &t.CreatedAt)
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

// ListTracksWithoutLoudness returns ready tracks that have a file but no loudness measurement yet.
func (q *Queries) ListTracksWithoutLoudness(ctx context.Context) ([]models.Track, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT id, file_path FROM tracks
		WHERE status = 'ready' AND loudness_lufs IS NULL AND file_path != ''
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Track
	for rows.Next() {
		var t models.Track
		if err := rows.Scan(&t.ID, &t.FilePath); err != nil {
			continue
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListTracksWithBadMetadata returns ready tracks that have near-zero duration or missing
// waveform data — typically MediaRecorder recordings uploaded before the re-probe fix.
func (q *Queries) ListTracksWithBadMetadata(ctx context.Context) ([]models.Track, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT id, file_path, format FROM tracks
		WHERE status = 'ready' AND (duration_ms <= 1 OR waveform_data IS NULL)
		  AND file_path != ''
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Track
	for rows.Next() {
		var t models.Track
		if err := rows.Scan(&t.ID, &t.FilePath, &t.Format); err != nil {
			continue
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (q *Queries) UpdateTrackError(ctx context.Context, id uuid.UUID) error {
	_, err := q.pool.Exec(ctx, "UPDATE tracks SET status = 'error' WHERE id = $1", id)
	return err
}

func (q *Queries) UpdateTrackLoudness(ctx context.Context, id uuid.UUID, lufs float64) error {
	_, err := q.pool.Exec(ctx, "UPDATE tracks SET loudness_lufs = $2 WHERE id = $1", id, lufs)
	return err
}

func (q *Queries) UpdateTrackGain(ctx context.Context, id uuid.UUID, gain float64) error {
	_, err := q.pool.Exec(ctx, "UPDATE tracks SET gain = $2 WHERE id = $1", id, gain)
	return err
}

func (q *Queries) UpdateTrackMuted(ctx context.Context, id uuid.UUID, muted bool) error {
	_, err := q.pool.Exec(ctx, "UPDATE tracks SET muted = $2 WHERE id = $1", id, muted)
	return err
}

// CountOverdubs returns the number of active (non-bounced) overdubs for a parent track.
func (q *Queries) CountOverdubs(ctx context.Context, parentID uuid.UUID) (int, error) {
	var n int
	err := q.pool.QueryRow(ctx,
		`SELECT count(*) FROM tracks WHERE overdub_of = $1 AND bounced_to IS NULL`,
		parentID,
	).Scan(&n)
	return n, err
}

// CountOverdubsForBand returns a map of parentID → active overdub count for
// all parent tracks in the band, in one query.
func (q *Queries) CountOverdubsForBand(ctx context.Context, bandID uuid.UUID) (map[uuid.UUID]int, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT overdub_of, count(*)
		FROM tracks
		WHERE band_id = $1 AND overdub_of IS NOT NULL AND bounced_to IS NULL
		GROUP BY overdub_of
	`, bandID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[uuid.UUID]int)
	for rows.Next() {
		var id uuid.UUID
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, nil
}

func (q *Queries) ResetTrackStatus(ctx context.Context, id uuid.UUID) error {
	_, err := q.pool.Exec(ctx, "UPDATE tracks SET status = 'processing' WHERE id = $1", id)
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
		       t.status, t.tags, t.notes, t.song_id, t.source_url,
		       t.recorded_at, t.set_id, t.overdub_of, t.offset_ms, t.bounced_to,
		       t.rpp_session_name, t.gain, t.muted, t.loudness_lufs, t.created_at,
		       u.id, u.email, u.display_name, u.avatar_url, u.is_admin, u.is_demo, u.created_at,
		       s.name
		FROM tracks t
		JOIN users u ON t.uploaded_by = u.id
		LEFT JOIN songs s ON t.song_id = s.id
		WHERE t.id = $1
	`, id).Scan(&t.ID, &t.BandID, &t.Title, &t.Description, &t.UploadedBy, &t.FilePath,
		&t.WaveformData, &t.DurationMS, &t.Format, &t.SampleRate, &t.FileSize,
		&t.Status, &t.Tags, &t.Notes, &songID, &t.SourceURL,
		&t.RecordedAt, &t.SetID, &t.OverdubOf, &t.OffsetMS, &t.BouncedTo,
		&t.RppSessionName, &t.Gain, &t.Muted, &t.LoudnessLUFS, &t.CreatedAt,
		&u.ID, &u.Email, &u.DisplayName, &u.AvatarURL, &u.IsAdmin, &u.IsDemo, &u.CreatedAt,
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
		       t.duration_ms, t.format, t.sample_rate, t.file_size,
		       t.status, t.tags, t.notes, t.song_id, t.source_url,
		       t.recorded_at, t.set_id, t.overdub_of, t.offset_ms, t.bounced_to,
		       t.gain, t.loudness_lufs, t.created_at,
		       u.id, u.email, u.display_name, u.avatar_url, u.is_admin, u.is_demo, u.created_at,
		       s.name
		FROM tracks t
		JOIN users u ON t.uploaded_by = u.id
		LEFT JOIN songs s ON t.song_id = s.id
		WHERE t.band_id = $1 AND t.overdub_of IS NULL and t.bounced_to IS NULL
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
			&t.DurationMS, &t.Format, &t.SampleRate, &t.FileSize,
			&t.Status, &t.Tags, &t.Notes, &songID, &t.SourceURL,
			&t.RecordedAt, &t.SetID, &t.OverdubOf, &t.OffsetMS, &t.BouncedTo,
			&t.Gain, &t.LoudnessLUFS, &t.CreatedAt,
			&u.ID, &u.Email, &u.DisplayName, &u.AvatarURL, &u.IsAdmin, &u.IsDemo, &u.CreatedAt,
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

func (q *Queries) UpdateTrackMeta(ctx context.Context, id uuid.UUID, title, description, notes string, recordedAt *time.Time) error {
	_, err := q.pool.Exec(ctx, `UPDATE tracks SET title = $2, description = $3, notes = $4, recorded_at = $5 WHERE id = $1`, id, title, description, notes, recordedAt)
	return err
}

func (q *Queries) UpdateTrackSet(ctx context.Context, trackID uuid.UUID, setID *uuid.UUID) error {
	_, err := q.pool.Exec(ctx, `UPDATE tracks SET set_id = $2 WHERE id = $1`, trackID, setID)
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

// IsFileShared returns true if more than one track row references the same file_path.
// Used before deleting a file to avoid nuking a file that a cloned overdub still points to.
func (q *Queries) IsFileShared(ctx context.Context, filePath string) (bool, error) {
	var count int
	err := q.pool.QueryRow(ctx, "SELECT COUNT(*) FROM tracks WHERE file_path = $1", filePath).Scan(&count)
	return count > 1, err
}

// CloneTrackAsOverdub creates a new track row that shares the source track's file and
// metadata but is attached as an overdub of parentID.
func (q *Queries) CloneTrackAsOverdub(ctx context.Context, sourceID, parentID uuid.UUID, offsetMS int64) (*models.Track, error) {
	var t models.Track
	err := q.pool.QueryRow(ctx, `
		INSERT INTO tracks (band_id, title, description, uploaded_by, file_path, file_size,
		                   waveform_data, duration_ms, format, sample_rate, status,
		                   tags, notes, source_url, recorded_at, overdub_of, offset_ms)
		SELECT band_id, title, description, uploaded_by, file_path, file_size,
		       waveform_data, duration_ms, format, sample_rate, status,
		       tags, notes, source_url, recorded_at, $2, $3
		FROM tracks WHERE id = $1
		RETURNING id, created_at
	`, sourceID, parentID, offsetMS).Scan(&t.ID, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// Track Personnel

func (q *Queries) ListTrackPersonnel(ctx context.Context, trackID uuid.UUID) ([]models.TrackPersonnel, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT tp.track_id, tp.user_id, tp.role,
		       u.id, u.email, u.display_name, u.avatar_url, u.is_admin, u.is_demo, u.created_at
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
			&u.ID, &u.Email, &u.DisplayName, &u.AvatarURL, &u.IsAdmin, &u.IsDemo, &u.CreatedAt); err != nil {
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
		       u.id, u.email, u.display_name, u.avatar_url, u.is_admin, u.is_demo, u.created_at
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
			&u.ID, &u.Email, &u.DisplayName, &u.AvatarURL, &u.IsAdmin, &u.IsDemo, &u.CreatedAt); err != nil {
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

// Sets

func (q *Queries) CreateSet(ctx context.Context, s *models.Set) error {
	return q.pool.QueryRow(ctx, `
		INSERT INTO sets (band_id, name, set_type, recorded_at, notes)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at
	`, s.BandID, s.Name, s.SetType, s.RecordedAt, s.Notes).Scan(&s.ID, &s.CreatedAt)
}

func (q *Queries) ListSets(ctx context.Context, bandID uuid.UUID) ([]models.Set, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT s.id, s.band_id, s.name, s.set_type, s.recorded_at, s.notes, s.created_at,
		       (SELECT count(*) FROM set_items WHERE set_id = s.id)
		FROM sets s
		WHERE s.band_id = $1
		ORDER BY COALESCE(s.recorded_at, s.created_at::date) DESC, s.created_at DESC
	`, bandID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sets []models.Set
	for rows.Next() {
		var s models.Set
		if err := rows.Scan(&s.ID, &s.BandID, &s.Name, &s.SetType, &s.RecordedAt, &s.Notes, &s.CreatedAt, &s.ItemCount); err != nil {
			return nil, err
		}
		sets = append(sets, s)
	}
	return sets, nil
}

func (q *Queries) GetSet(ctx context.Context, id uuid.UUID) (*models.Set, error) {
	var s models.Set
	err := q.pool.QueryRow(ctx, `
		SELECT id, band_id, name, set_type, recorded_at, notes, created_at
		FROM sets WHERE id = $1
	`, id).Scan(&s.ID, &s.BandID, &s.Name, &s.SetType, &s.RecordedAt, &s.Notes, &s.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &s, err
}

func (q *Queries) UpdateSet(ctx context.Context, id uuid.UUID, name, setType string, recordedAt *time.Time, notes string) error {
	_, err := q.pool.Exec(ctx, `
		UPDATE sets SET name = $2, set_type = $3, recorded_at = $4, notes = $5
		WHERE id = $1
	`, id, name, setType, recordedAt, notes)
	return err
}

func (q *Queries) DeleteSet(ctx context.Context, id uuid.UUID) error {
	_, err := q.pool.Exec(ctx, `DELETE FROM sets WHERE id = $1`, id)
	return err
}

func (q *Queries) ListSetItems(ctx context.Context, setID uuid.UUID) ([]models.SetItem, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT si.id, si.set_id, si.position, si.song_id, si.custom_name,
		       si.start_ms, si.end_ms, si.notes, COALESCE(s.name, '')
		FROM set_items si
		LEFT JOIN songs s ON si.song_id = s.id
		WHERE si.set_id = $1
		ORDER BY si.position
	`, setID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []models.SetItem
	for rows.Next() {
		var si models.SetItem
		if err := rows.Scan(&si.ID, &si.SetID, &si.Position, &si.SongID, &si.CustomName,
			&si.StartMS, &si.EndMS, &si.Notes, &si.SongName); err != nil {
			return nil, err
		}
		items = append(items, si)
	}
	return items, nil
}

func (q *Queries) ListPerformItems(ctx context.Context, setID uuid.UUID) ([]models.PerformItem, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT si.position, COALESCE(s.name, ''), si.custom_name,
		       COALESCE(s.lyrics, ''), COALESCE(s.tabs, ''), si.notes
		FROM set_items si
		LEFT JOIN songs s ON si.song_id = s.id
		WHERE si.set_id = $1
		ORDER BY si.position
	`, setID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []models.PerformItem
	for rows.Next() {
		var pi models.PerformItem
		if err := rows.Scan(&pi.Position, &pi.SongName, &pi.CustomName,
			&pi.Lyrics, &pi.Tabs, &pi.Notes); err != nil {
			return nil, err
		}
		items = append(items, pi)
	}
	return items, nil
}

func (q *Queries) ReplaceSetItems(ctx context.Context, setID uuid.UUID, items []models.SetItem) ([]models.SetItem, error) {
	tx, err := q.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `DELETE FROM set_items WHERE set_id = $1`, setID)
	if err != nil {
		return nil, err
	}

	var result []models.SetItem
	for i, item := range items {
		var si models.SetItem
		err := tx.QueryRow(ctx, `
			INSERT INTO set_items (set_id, position, song_id, custom_name, start_ms, end_ms, notes)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id, set_id, position, song_id, custom_name, start_ms, end_ms, notes
		`, setID, i, item.SongID, item.CustomName, item.StartMS, item.EndMS, item.Notes).Scan(
			&si.ID, &si.SetID, &si.Position, &si.SongID, &si.CustomName, &si.StartMS, &si.EndMS, &si.Notes)
		if err != nil {
			return nil, err
		}
		result = append(result, si)
	}

	return result, tx.Commit(ctx)
}

func (q *Queries) ListSetTracks(ctx context.Context, setID uuid.UUID) ([]models.Track, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT t.id, t.band_id, t.title, t.description, t.uploaded_by, t.file_path,
		       t.waveform_data, t.duration_ms, t.format, t.sample_rate, t.file_size,
		       t.status, t.tags, t.notes, t.song_id, t.source_url,
		       t.recorded_at, t.set_id, t.overdub_of, t.offset_ms, t.bounced_to,
		       t.gain, t.loudness_lufs, t.created_at,
		       u.id, u.email, u.display_name, u.avatar_url, u.is_admin, u.is_demo, u.created_at,
		       s.name
		FROM tracks t
		JOIN users u ON t.uploaded_by = u.id
		LEFT JOIN songs s ON t.song_id = s.id
		WHERE t.set_id = $1 AND t.overdub_of IS NULL
		ORDER BY t.created_at DESC
	`, setID)
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
			&t.Status, &t.Tags, &t.Notes, &songID, &t.SourceURL,
			&t.RecordedAt, &t.SetID, &t.OverdubOf, &t.OffsetMS, &t.BouncedTo,
			&t.Gain, &t.LoudnessLUFS, &t.CreatedAt,
			&u.ID, &u.Email, &u.DisplayName, &u.AvatarURL, &u.IsAdmin, &u.IsDemo, &u.CreatedAt,
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

func (q *Queries) ListSetTakesBySong(ctx context.Context, bandID, songID uuid.UUID) ([]models.SetTake, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT si.id, s.id, s.name, s.set_type, si.start_ms, si.end_ms,
		       t.id, t.title, t.duration_ms, t.format, t.file_size,
		       t.tags, t.recorded_at, t.created_at,
		       u.id, u.email, u.display_name, u.avatar_url, u.is_admin, u.is_demo, u.created_at
		FROM set_items si
		JOIN sets s ON si.set_id = s.id
		JOIN tracks t ON t.set_id = s.id AND t.status = 'ready'
		JOIN users u ON t.uploaded_by = u.id
		WHERE si.song_id = $1 AND s.band_id = $2
		  AND si.start_ms IS NOT NULL AND si.end_ms IS NOT NULL
		ORDER BY COALESCE(s.recorded_at, s.created_at::date) DESC
	`, songID, bandID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var takes []models.SetTake
	for rows.Next() {
		var st models.SetTake
		var u models.User
		if err := rows.Scan(&st.SetItemID, &st.SetID, &st.SetName, &st.SetType, &st.StartMS, &st.EndMS,
			&st.TrackID, &st.TrackTitle, &st.DurationMS, &st.Format, &st.FileSize,
			&st.Tags, &st.RecordedAt, &st.CreatedAt,
			&u.ID, &u.Email, &u.DisplayName, &u.AvatarURL, &u.IsAdmin, &u.IsDemo, &u.CreatedAt); err != nil {
			return nil, err
		}
		st.Uploader = &u
		takes = append(takes, st)
	}
	return takes, nil
}

func (q *Queries) ListSetsBySong(ctx context.Context, bandID, songID uuid.UUID) ([]models.Set, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT s.id, s.band_id, s.name, s.set_type, s.recorded_at, s.notes, s.created_at,
		       (SELECT count(*) FROM set_items WHERE set_id = s.id)
		FROM sets s
		JOIN set_items si ON si.set_id = s.id
		WHERE s.band_id = $1 AND si.song_id = $2
		GROUP BY s.id
		ORDER BY COALESCE(s.recorded_at, s.created_at::date) DESC
	`, bandID, songID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sets []models.Set
	for rows.Next() {
		var s models.Set
		if err := rows.Scan(&s.ID, &s.BandID, &s.Name, &s.SetType, &s.RecordedAt, &s.Notes, &s.CreatedAt, &s.ItemCount); err != nil {
			return nil, err
		}
		sets = append(sets, s)
	}
	return sets, nil
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

// Feedback

func (q *Queries) CreateFeedback(ctx context.Context, fb *models.Feedback) error {
	return q.pool.QueryRow(ctx, `
		INSERT INTO feedback (user_id, body, page_url, image_path)
		VALUES ($1, $2, $3, $4)
		RETURNING id, status, created_at
	`, fb.UserID, fb.Body, fb.PageURL, fb.ImagePath).Scan(&fb.ID, &fb.Status, &fb.CreatedAt)
}

func (q *Queries) ListFeedback(ctx context.Context) ([]models.Feedback, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT f.id, f.user_id, f.body, f.page_url, f.image_path, f.status, f.created_at,
		       u.id, u.email, u.display_name, u.avatar_url, u.is_admin, u.is_demo, u.created_at
		FROM feedback f
		JOIN users u ON f.user_id = u.id
		ORDER BY f.created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []models.Feedback
	for rows.Next() {
		var fb models.Feedback
		var u models.User
		if err := rows.Scan(&fb.ID, &fb.UserID, &fb.Body, &fb.PageURL, &fb.ImagePath, &fb.Status, &fb.CreatedAt,
			&u.ID, &u.Email, &u.DisplayName, &u.AvatarURL, &u.IsAdmin, &u.IsDemo, &u.CreatedAt); err != nil {
			return nil, err
		}
		fb.HasImage = fb.ImagePath != ""
		fb.User = &u
		items = append(items, fb)
	}
	return items, nil
}

func (q *Queries) UpdateFeedbackStatus(ctx context.Context, id uuid.UUID, status string) error {
	_, err := q.pool.Exec(ctx, `UPDATE feedback SET status = $2 WHERE id = $1`, id, status)
	return err
}

func (q *Queries) GetFeedbackImagePath(ctx context.Context, id uuid.UUID) (string, error) {
	var path string
	err := q.pool.QueryRow(ctx, `SELECT image_path FROM feedback WHERE id = $1`, id).Scan(&path)
	return path, err
}

// Admin

func (q *Queries) ListAllUsers(ctx context.Context) ([]models.AdminUser, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT u.id, u.email, u.display_name, u.avatar_url, u.is_admin, u.is_demo, u.created_at,
		       COALESCE(
		         (SELECT string_agg(b.name, ', ' ORDER BY b.name)
		          FROM band_members bm JOIN bands b ON bm.band_id = b.id
		          WHERE bm.user_id = u.id), ''
		       )
		FROM users u
		ORDER BY u.created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []models.AdminUser
	for rows.Next() {
		var u models.AdminUser
		if err := rows.Scan(&u.ID, &u.Email, &u.DisplayName, &u.AvatarURL, &u.IsAdmin, &u.IsDemo, &u.CreatedAt, &u.Bands); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func (q *Queries) ListAllBands(ctx context.Context) ([]models.AdminBand, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT b.id, b.name, b.slug, b.color_scheme, b.created_at,
		       (SELECT count(*) FROM band_members WHERE band_id = b.id),
		       (SELECT count(*) FROM tracks WHERE band_id = b.id)
		FROM bands b
		ORDER BY b.created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bands []models.AdminBand
	for rows.Next() {
		var b models.AdminBand
		if err := rows.Scan(&b.ID, &b.Name, &b.Slug, &b.ColorScheme, &b.CreatedAt, &b.MemberCount, &b.TrackCount); err != nil {
			return nil, err
		}
		bands = append(bands, b)
	}
	return bands, nil
}

func (q *Queries) ListAllInvites(ctx context.Context) ([]models.AdminInvite, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT i.id, i.email, i.token, i.expires_at, i.used_by, i.used_at, i.created_at,
		       b.name,
		       creator.display_name,
		       acceptor.display_name
		FROM band_invites i
		JOIN bands b ON i.band_id = b.id
		JOIN users creator ON i.created_by = creator.id
		LEFT JOIN users acceptor ON i.used_by = acceptor.id
		ORDER BY i.created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var invites []models.AdminInvite
	for rows.Next() {
		var inv models.AdminInvite
		if err := rows.Scan(&inv.ID, &inv.Email, &inv.Token, &inv.ExpiresAt, &inv.UsedBy, &inv.UsedAt, &inv.CreatedAt,
			&inv.BandName, &inv.CreatedByName, &inv.UsedByName); err != nil {
			return nil, err
		}
		invites = append(invites, inv)
	}
	return invites, nil
}

// Overdubs

func (q *Queries) ListOverdubs(ctx context.Context, parentID, viewerID uuid.UUID) ([]models.Track, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT t.id, t.band_id, t.title, t.description, t.uploaded_by, t.file_path,
		       t.waveform_data, t.duration_ms, t.format, t.sample_rate, t.file_size,
		       t.status, t.tags, t.notes, t.song_id, t.source_url,
		       t.recorded_at, t.set_id, t.overdub_of, t.offset_ms, t.bounced_to,
		       t.gain, t.muted, t.loudness_lufs, t.created_at,
		       u.id, u.email, u.display_name, u.avatar_url, u.is_admin, u.is_demo, u.created_at,
		       (SELECT count(*) FROM overdub_votes WHERE overdub_id = t.id),
		       EXISTS(SELECT 1 FROM overdub_votes WHERE overdub_id = t.id AND user_id = $2)
		FROM tracks t
		JOIN users u ON t.uploaded_by = u.id
		WHERE t.overdub_of = $1 AND t.bounced_to IS NULL
		ORDER BY t.created_at ASC
	`, parentID, viewerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tracks []models.Track
	for rows.Next() {
		var t models.Track
		var u models.User
		var songID *uuid.UUID
		if err := rows.Scan(&t.ID, &t.BandID, &t.Title, &t.Description, &t.UploadedBy, &t.FilePath,
			&t.WaveformData, &t.DurationMS, &t.Format, &t.SampleRate, &t.FileSize,
			&t.Status, &t.Tags, &t.Notes, &songID, &t.SourceURL,
			&t.RecordedAt, &t.SetID, &t.OverdubOf, &t.OffsetMS, &t.BouncedTo,
			&t.Gain, &t.Muted, &t.LoudnessLUFS, &t.CreatedAt,
			&u.ID, &u.Email, &u.DisplayName, &u.AvatarURL, &u.IsAdmin, &u.IsDemo, &u.CreatedAt,
			&t.VoteCount, &t.UserVoted); err != nil {
			return nil, err
		}
		t.Uploader = &u
		t.SongID = songID
		tracks = append(tracks, t)
	}
	return tracks, nil
}

func (q *Queries) SetOverdubOf(ctx context.Context, trackID uuid.UUID, parentID *uuid.UUID, offsetMS int64) error {
	_, err := q.pool.Exec(ctx, `
		UPDATE tracks SET overdub_of = $2, offset_ms = $3 WHERE id = $1
	`, trackID, parentID, offsetMS)
	return err
}

func (q *Queries) VoteOverdub(ctx context.Context, parentID, userID, overdubID uuid.UUID) error {
	_, err := q.pool.Exec(ctx, `
		INSERT INTO overdub_votes (track_id, user_id, overdub_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (track_id, user_id) DO UPDATE SET overdub_id = $3, created_at = now()
	`, parentID, userID, overdubID)
	return err
}

func (q *Queries) UnvoteOverdub(ctx context.Context, parentID, userID uuid.UUID) error {
	_, err := q.pool.Exec(ctx, `
		DELETE FROM overdub_votes WHERE track_id = $1 AND user_id = $2
	`, parentID, userID)
	return err
}

func (q *Queries) GetPreBounceID(ctx context.Context, trackID uuid.UUID) *uuid.UUID {
	var id uuid.UUID
	err := q.pool.QueryRow(ctx, `SELECT id FROM tracks WHERE bounced_to = $1 AND overdub_of IS NULL ORDER BY created_at ASC LIMIT 1`, trackID).Scan(&id)
	if err != nil {
		return nil
	}
	return &id
}

func (q *Queries) CountBounceVersions(ctx context.Context, trackID uuid.UUID) int {
	var count int
	q.pool.QueryRow(ctx, `SELECT count(*) FROM tracks WHERE bounced_to = $1 AND overdub_of IS NULL`, trackID).Scan(&count)
	return count
}

func (q *Queries) ListBounceVersions(ctx context.Context, trackID uuid.UUID) ([]models.Track, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT id, title, file_path, file_size, duration_ms, created_at
		FROM tracks WHERE bounced_to = $1 AND overdub_of IS NULL
		ORDER BY created_at ASC
	`, trackID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tracks []models.Track
	for rows.Next() {
		var t models.Track
		if err := rows.Scan(&t.ID, &t.Title, &t.FilePath, &t.FileSize, &t.DurationMS, &t.CreatedAt); err != nil {
			return nil, err
		}
		tracks = append(tracks, t)
	}
	return tracks, nil
}

func (q *Queries) ClearBouncedOverdubs(ctx context.Context, parentID uuid.UUID) error {
	_, err := q.pool.Exec(ctx, `UPDATE tracks SET bounced_to = NULL WHERE bounced_to = $1 AND overdub_of = $1`, parentID)
	return err
}

func (q *Queries) UpdateBouncedTo(ctx context.Context, trackID uuid.UUID, bouncedTo *uuid.UUID) error {
	_, err := q.pool.Exec(ctx, `UPDATE tracks SET bounced_to = $2 WHERE id = $1`, trackID, bouncedTo)
	return err
}

func (q *Queries) UpdateOverdubOffset(ctx context.Context, trackID uuid.UUID, offsetMS int64) error {
	_, err := q.pool.Exec(ctx, `UPDATE tracks SET offset_ms = $2 WHERE id = $1 AND overdub_of IS NOT NULL`, trackID, offsetMS)
	return err
}

func (q *Queries) DeleteOverdubVotes(ctx context.Context, parentID uuid.UUID) error {
	_, err := q.pool.Exec(ctx, `
		DELETE FROM overdub_votes WHERE track_id = $1
	`, parentID)
	return err
}

// Activity / Unread

func (q *Queries) UpdateLastSeen(ctx context.Context, bandID, userID uuid.UUID) error {
	_, err := q.pool.Exec(ctx, `
		UPDATE band_members SET last_seen_at = now()
		WHERE band_id = $1 AND user_id = $2
	`, bandID, userID)
	return err
}

func (q *Queries) GetActivityFeed(ctx context.Context, userID uuid.UUID, limit int) ([]models.ActivityItem, error) {
	rows, err := q.pool.Query(ctx, `
		WITH user_bands AS (
			SELECT bm.band_id, bm.last_seen_at, b.slug, b.name
			FROM band_members bm
			JOIN bands b ON b.id = bm.band_id
			WHERE bm.user_id = $1
		)
		(
			SELECT 'track' AS type, u.display_name AS actor, t.title AS subject,
			       ub.slug, ub.name, t.id::text AS link_id, t.created_at
			FROM tracks t
			JOIN users u ON t.uploaded_by = u.id
			JOIN user_bands ub ON t.band_id = ub.band_id
			WHERE t.created_at > ub.last_seen_at AND t.uploaded_by != $1 AND t.overdub_of IS NULL
		) UNION ALL (
			SELECT 'comment', u.display_name, t.title,
			       ub.slug, ub.name, t.id::text, c.created_at
			FROM comments c
			JOIN users u ON c.user_id = u.id
			JOIN tracks t ON c.track_id = t.id
			JOIN user_bands ub ON t.band_id = ub.band_id
			WHERE c.created_at > ub.last_seen_at AND c.user_id != $1
		) UNION ALL (
			SELECT 'chat', u.display_name, LEFT(m.body, 60),
			       ub.slug, ub.name, '', m.created_at
			FROM chat_messages m
			JOIN users u ON m.user_id = u.id
			JOIN user_bands ub ON m.band_id = ub.band_id
			WHERE m.created_at > ub.last_seen_at AND m.user_id != $1
		) UNION ALL (
			SELECT 'song', '', s.name,
			       ub.slug, ub.name, '', s.created_at
			FROM songs s
			JOIN user_bands ub ON s.band_id = ub.band_id
			WHERE s.created_at > ub.last_seen_at
		)
		ORDER BY created_at DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []models.ActivityItem
	for rows.Next() {
		var a models.ActivityItem
		if err := rows.Scan(&a.Type, &a.ActorName, &a.Subject, &a.BandSlug, &a.BandName, &a.LinkID, &a.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, a)
	}
	return items, nil
}

func downsamplePeaks(peaks []float64, n int) []float64 {
	if len(peaks) == 0 {
		return nil
	}
	if len(peaks) <= n {
		return peaks
	}
	out := make([]float64, n)
	for i := range out {
		out[i] = peaks[i*len(peaks)/n]
	}
	return out
}

func (q *Queries) GetBandActivity(ctx context.Context, bandID, userID uuid.UUID, limit int) ([]models.ActivityItem, error) {
	rows, err := q.pool.Query(ctx, `
		WITH last_seen AS (
			SELECT COALESCE(last_seen_at, '-infinity'::timestamptz) AS ts
			FROM band_members WHERE band_id = $1 AND user_id = $2
		)
		SELECT type, actor_name, subject, link_id, stream_id, created_at,
		       waveform_data, duration_ms, preview, timestamp_ms,
		       COALESCE(created_at > (SELECT ts FROM last_seen), false) AS is_new
		FROM (
			SELECT 'track' AS type, u.display_name AS actor_name, t.title AS subject,
			       t.id::text AS link_id, t.id::text AS stream_id, t.created_at,
			       t.waveform_data, t.duration_ms, NULL::text AS preview, NULL::bigint AS timestamp_ms
			FROM tracks t
			JOIN users u ON t.uploaded_by = u.id
			WHERE t.band_id = $1 AND t.overdub_of IS NULL AND t.bounced_to IS NULL
		UNION ALL
			SELECT 'overdub', u.display_name, p.title,
			       p.id::text, t.id::text, t.created_at,
			       t.waveform_data, t.duration_ms, NULL::text, NULL::bigint
			FROM tracks t
			JOIN users u ON t.uploaded_by = u.id
			JOIN tracks p ON t.overdub_of = p.id
			WHERE t.band_id = $1
		UNION ALL
			SELECT 'comment', u.display_name, t.title,
			       t.id::text, ''::text, c.created_at,
			       NULL::jsonb, NULL::bigint, LEFT(c.body, 140), c.timestamp_ms
			FROM comments c
			JOIN users u ON c.user_id = u.id
			JOIN tracks t ON c.track_id = t.id
			WHERE t.band_id = $1
		UNION ALL
			SELECT 'song', '', s.name,
			       s.id::text, ''::text, s.created_at,
			       NULL::jsonb, NULL::bigint, NULL::text, NULL::bigint
			FROM songs s
			WHERE s.band_id = $1
		) q
		ORDER BY created_at DESC
		LIMIT $3
	`, bandID, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []models.ActivityItem
	for rows.Next() {
		var a models.ActivityItem
		var rawWaveform []byte
		var durMs, tsMs *int64
		var preview *string
		if err := rows.Scan(
			&a.Type, &a.ActorName, &a.Subject, &a.LinkID, &a.StreamID, &a.CreatedAt,
			&rawWaveform, &durMs, &preview, &tsMs, &a.IsNew,
		); err != nil {
			return nil, err
		}
		if rawWaveform != nil {
			var peaks []float64
			if json.Unmarshal(rawWaveform, &peaks) == nil {
				a.Peaks = downsamplePeaks(peaks, 60)
			}
		}
		if durMs != nil {
			a.DurationMs = *durMs
		}
		if preview != nil {
			a.Preview = *preview
		}
		a.TimestampMs = tsMs
		items = append(items, a)
	}
	return items, nil
}

// API tokens

func (q *Queries) GetOrCreateAPIToken(ctx context.Context, userID, bandID uuid.UUID) (*models.APIToken, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	newToken := hex.EncodeToString(b)

	var t models.APIToken
	err := q.pool.QueryRow(ctx, `
		INSERT INTO api_tokens (user_id, band_id, token)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, band_id) DO UPDATE SET token = api_tokens.token
		RETURNING id, user_id, band_id, token, created_at, last_used_at
	`, userID, bandID, newToken).Scan(&t.ID, &t.UserID, &t.BandID, &t.Token, &t.CreatedAt, &t.LastUsedAt)
	return &t, err
}

func (q *Queries) RotateAPIToken(ctx context.Context, userID, bandID uuid.UUID) (*models.APIToken, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	newToken := hex.EncodeToString(b)

	var t models.APIToken
	err := q.pool.QueryRow(ctx, `
		INSERT INTO api_tokens (user_id, band_id, token)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, band_id) DO UPDATE SET token = EXCLUDED.token
		RETURNING id, user_id, band_id, token, created_at, last_used_at
	`, userID, bandID, newToken).Scan(&t.ID, &t.UserID, &t.BandID, &t.Token, &t.CreatedAt, &t.LastUsedAt)
	return &t, err
}

func (q *Queries) GetUserByToken(ctx context.Context, token string) (*models.User, error) {
	var u models.User
	err := q.pool.QueryRow(ctx, `
		UPDATE api_tokens SET last_used_at = now()
		WHERE token = $1
		RETURNING user_id
	`, token).Scan(&u.ID)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return q.GetUser(ctx, u.ID)
}

// Reaper sync

func (q *Queries) GetOrCreateRppSession(ctx context.Context, bandID, userID uuid.UUID, sessionName, songID string) (*models.Track, error) {
	var t models.Track
	err := q.pool.QueryRow(ctx, `
		SELECT id, band_id, title, rpp_session_name, created_at
		FROM tracks WHERE band_id = $1 AND rpp_session_name = $2
		LIMIT 1
	`, bandID, sessionName).Scan(&t.ID, &t.BandID, &t.Title, &t.RppSessionName, &t.CreatedAt)
	if err == nil {
		return &t, nil
	}
	if err != pgx.ErrNoRows {
		return nil, err
	}

	// Parse optional song ID
	var sid *uuid.UUID
	if songID != "" {
		if parsed, err := uuid.Parse(songID); err == nil {
			sid = &parsed
		}
	}

	err = q.pool.QueryRow(ctx, `
		INSERT INTO tracks (band_id, title, uploaded_by, file_path, file_size, status, rpp_session_name, song_id)
		VALUES ($1, $2, $3, '', 0, 'ready', $2, $4)
		RETURNING id, band_id, title, rpp_session_name, created_at
	`, bandID, sessionName, userID, sid).Scan(&t.ID, &t.BandID, &t.Title, &t.RppSessionName, &t.CreatedAt)
	return &t, err
}

func (q *Queries) GetSyncState(ctx context.Context, trackID uuid.UUID) ([]models.SyncFile, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT filename, file_hash, overdub_id FROM sync_files WHERE track_id = $1
	`, trackID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []models.SyncFile
	for rows.Next() {
		var f models.SyncFile
		if err := rows.Scan(&f.Filename, &f.FileHash, &f.OverdubID); err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, nil
}

func (q *Queries) UpsertSyncFile(ctx context.Context, trackID uuid.UUID, overdubID *uuid.UUID, filename, hash string) error {
	_, err := q.pool.Exec(ctx, `
		INSERT INTO sync_files (track_id, overdub_id, filename, file_hash)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (track_id, filename) DO UPDATE SET overdub_id = $2, file_hash = $4
	`, trackID, overdubID, filename, hash)
	return err
}

func (q *Queries) CreateRppVersion(ctx context.Context, trackID uuid.UUID, storageKey string) (*models.RppVersion, error) {
	var v models.RppVersion
	err := q.pool.QueryRow(ctx, `
		INSERT INTO rpp_versions (track_id, storage_key, version)
		VALUES ($1, $2, COALESCE((SELECT MAX(version) FROM rpp_versions WHERE track_id = $1), 0) + 1)
		RETURNING id, track_id, storage_key, version, created_at
	`, trackID, storageKey).Scan(&v.ID, &v.TrackID, &v.StorageKey, &v.Version, &v.CreatedAt)
	return &v, err
}

type SyncSession struct {
	TrackID     uuid.UUID `json:"track_id"`
	SessionName string    `json:"session_name"`
}

func (q *Queries) ListSyncSessions(ctx context.Context, bandID uuid.UUID) ([]SyncSession, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT id, rpp_session_name FROM tracks
		WHERE band_id = $1 AND rpp_session_name IS NOT NULL AND rpp_session_name <> ''
		ORDER BY created_at DESC
	`, bandID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sessions []SyncSession
	for rows.Next() {
		var s SyncSession
		if err := rows.Scan(&s.TrackID, &s.SessionName); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

func (q *Queries) GetLatestRppVersion(ctx context.Context, trackID uuid.UUID) (*models.RppVersion, error) {
	var v models.RppVersion
	err := q.pool.QueryRow(ctx, `
		SELECT id, track_id, storage_key, version, created_at
		FROM rpp_versions WHERE track_id = $1 ORDER BY version DESC LIMIT 1
	`, trackID).Scan(&v.ID, &v.TrackID, &v.StorageKey, &v.Version, &v.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &v, err
}

// Band files

func (q *Queries) CreateBandFile(ctx context.Context, f *models.BandFile) error {
	return q.pool.QueryRow(ctx, `
		INSERT INTO band_files (band_id, name, storage_key, file_size, content_type, uploaded_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at
	`, f.BandID, f.Name, f.StorageKey, f.FileSize, f.ContentType, f.UploadedBy).Scan(&f.ID, &f.CreatedAt)
}

func (q *Queries) ListBandFiles(ctx context.Context, bandID uuid.UUID) ([]models.BandFile, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT f.id, f.band_id, f.name, f.storage_key, f.file_size, f.content_type, f.uploaded_by, f.created_at,
		       u.id, u.display_name, u.email
		FROM band_files f
		JOIN users u ON u.id = f.uploaded_by
		WHERE f.band_id = $1
		ORDER BY f.created_at DESC
	`, bandID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []models.BandFile
	for rows.Next() {
		var f models.BandFile
		var u models.User
		if err := rows.Scan(&f.ID, &f.BandID, &f.Name, &f.StorageKey, &f.FileSize, &f.ContentType, &f.UploadedBy, &f.CreatedAt,
			&u.ID, &u.DisplayName, &u.Email); err != nil {
			return nil, err
		}
		f.Uploader = &u
		files = append(files, f)
	}
	return files, nil
}

func (q *Queries) GetBandFile(ctx context.Context, id, bandID uuid.UUID) (*models.BandFile, error) {
	var f models.BandFile
	err := q.pool.QueryRow(ctx, `
		SELECT id, band_id, name, storage_key, file_size, content_type, uploaded_by, created_at
		FROM band_files WHERE id = $1 AND band_id = $2
	`, id, bandID).Scan(&f.ID, &f.BandID, &f.Name, &f.StorageKey, &f.FileSize, &f.ContentType, &f.UploadedBy, &f.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &f, err
}

func (q *Queries) DeleteBandFile(ctx context.Context, id, bandID uuid.UUID) (string, error) {
	var key string
	err := q.pool.QueryRow(ctx, `
		DELETE FROM band_files WHERE id = $1 AND band_id = $2 RETURNING storage_key
	`, id, bandID).Scan(&key)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return key, err
}

func (q *Queries) GetUnreadCounts(ctx context.Context, userID uuid.UUID) ([]models.UnreadCount, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT bm.band_id, b.slug,
			(SELECT count(*) FROM tracks t WHERE t.band_id = bm.band_id AND t.created_at > bm.last_seen_at AND t.uploaded_by != $1 AND t.overdub_of IS NULL) AS new_tracks,
			(SELECT count(*) FROM comments c JOIN tracks t ON c.track_id = t.id WHERE t.band_id = bm.band_id AND c.created_at > bm.last_seen_at AND c.user_id != $1) AS new_comments,
			(SELECT count(*) FROM chat_messages m WHERE m.band_id = bm.band_id AND m.created_at > bm.last_seen_at AND m.user_id != $1) AS new_chats,
			(SELECT count(*) FROM songs s WHERE s.band_id = bm.band_id AND s.created_at > bm.last_seen_at) AS new_songs
		FROM band_members bm
		JOIN bands b ON b.id = bm.band_id
		WHERE bm.user_id = $1
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var counts []models.UnreadCount
	for rows.Next() {
		var c models.UnreadCount
		if err := rows.Scan(&c.BandID, &c.BandSlug, &c.NewTracks, &c.NewComments, &c.NewChats, &c.NewSongs); err != nil {
			return nil, err
		}
		c.Total = c.NewTracks + c.NewComments + c.NewChats + c.NewSongs
		counts = append(counts, c)
	}
	return counts, nil
}

// Events (analytics)

type EventInput struct {
	SessionID string
	UserID    *uuid.UUID
	Kind      string
	Path      string
	Metadata  json.RawMessage
	CreatedAt time.Time
}

func (q *Queries) InsertEvents(ctx context.Context, events []EventInput) error {
	if len(events) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, e := range events {
		meta := e.Metadata
		if len(meta) == 0 {
			meta = json.RawMessage(`{}`)
		}
		batch.Queue(
			`INSERT INTO events (session_id, user_id, kind, path, metadata, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			e.SessionID, e.UserID, e.Kind, e.Path, meta, e.CreatedAt,
		)
	}
	br := q.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range events {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (q *Queries) ListEventSessions(ctx context.Context, limit int) ([]models.EventSession, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := q.pool.Query(ctx, `
		SELECT e.session_id, e.user_id,
		       COALESCE(u.display_name, ''), COALESCE(u.email, ''),
		       MIN(e.created_at), MAX(e.created_at), COUNT(*)
		FROM events e
		LEFT JOIN users u ON e.user_id = u.id
		GROUP BY e.session_id, e.user_id, u.display_name, u.email
		ORDER BY MAX(e.created_at) DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []models.EventSession
	for rows.Next() {
		var s models.EventSession
		if err := rows.Scan(&s.SessionID, &s.UserID, &s.DisplayName, &s.Email, &s.FirstSeen, &s.LastSeen, &s.EventCount); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

func (q *Queries) ListEventsBySession(ctx context.Context, sessionID string) ([]models.Event, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT id, session_id, user_id, kind, path, metadata, created_at
		FROM events
		WHERE session_id = $1
		ORDER BY created_at ASC, id ASC
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []models.Event
	for rows.Next() {
		var e models.Event
		if err := rows.Scan(&e.ID, &e.SessionID, &e.UserID, &e.Kind, &e.Path, &e.Metadata, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, nil
}

func (q *Queries) EventKindCounts(ctx context.Context, since time.Time) (map[string]int, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT kind, COUNT(*) FROM events WHERE created_at >= $1 GROUP BY kind ORDER BY COUNT(*) DESC
	`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var k string
		var c int
		if err := rows.Scan(&k, &c); err != nil {
			return nil, err
		}
		out[k] = c
	}
	return out, nil
}
