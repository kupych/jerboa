package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID          uuid.UUID `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	AvatarURL   string    `json:"avatar_url,omitempty"`
	IsAdmin     bool      `json:"is_admin"`
	IsDemo      bool      `json:"is_demo"`
	CreatedAt   time.Time `json:"created_at"`
}

type Band struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	CreatedBy   uuid.UUID `json:"created_by"`
	ColorScheme string    `json:"color_scheme"`
	CreatedAt   time.Time `json:"created_at"`
}

type BandMember struct {
	BandID   uuid.UUID `json:"band_id"`
	UserID   uuid.UUID `json:"user_id"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
	User     *User     `json:"user,omitempty"`
}

type BandWithRole struct {
	Band
	Role string `json:"role"`
}

type BandInvite struct {
	ID        uuid.UUID  `json:"id"`
	BandID    uuid.UUID  `json:"band_id"`
	Token     string     `json:"token"`
	CreatedBy uuid.UUID  `json:"created_by"`
	ExpiresAt time.Time  `json:"expires_at"`
	UsedBy    *uuid.UUID `json:"used_by,omitempty"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type PendingInvite struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

type Song struct {
	ID        uuid.UUID `json:"id"`
	BandID    uuid.UUID `json:"band_id"`
	Name      string    `json:"name"`
	Lyrics    string    `json:"lyrics"`
	Tabs      string    `json:"tabs"`
	Notes     string    `json:"notes"`
	BPM       int       `json:"bpm"`
	CreatedAt time.Time `json:"created_at"`
	TakeCount int       `json:"take_count"`
}

type TrackPersonnel struct {
	TrackID uuid.UUID `json:"track_id"`
	UserID  uuid.UUID `json:"user_id"`
	Role    string    `json:"role"`
	User    *User     `json:"user,omitempty"`
}

type Track struct {
	ID           uuid.UUID       `json:"id"`
	BandID       uuid.UUID       `json:"band_id"`
	Title        string          `json:"title"`
	Description  string          `json:"description,omitempty"`
	UploadedBy   uuid.UUID       `json:"uploaded_by"`
	FilePath     string          `json:"-"`
	WaveformData json.RawMessage `json:"waveform_data,omitempty"`
	DurationMS   int64           `json:"duration_ms"`
	Format       string          `json:"format"`
	SampleRate   int             `json:"sample_rate"`
	FileSize     int64           `json:"file_size"`
	Status       string          `json:"status"`
	Tags         []string        `json:"tags"`
	Notes        string          `json:"notes,omitempty"`
	SongID       *uuid.UUID      `json:"song_id,omitempty"`
	SourceURL    string          `json:"source_url,omitempty"`
	RecordedAt   *time.Time      `json:"recorded_at,omitempty"`
	SetID        *uuid.UUID      `json:"set_id,omitempty"`
	OverdubOf    *uuid.UUID      `json:"overdub_of,omitempty"`
	OffsetMS     int64           `json:"offset_ms"`
	BouncedTo    *uuid.UUID     `json:"bounced_to,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	Uploader     *User             `json:"uploader,omitempty"`
	Song         *Song             `json:"song,omitempty"`
	Personnel    []TrackPersonnel  `json:"personnel,omitempty"`
	Overdubs     []Track           `json:"overdubs,omitempty"`
	VoteCount    int               `json:"vote_count,omitempty"`
	UserVoted    bool              `json:"user_voted,omitempty"`
	PreBounceID    *uuid.UUID        `json:"pre_bounce_id,omitempty"`
	BounceVersions int               `json:"bounce_versions,omitempty"`
	RppSessionName *string           `json:"rpp_session_name,omitempty"`
	Gain           float64           `json:"gain"`
	Muted          bool              `json:"muted"`
	LoudnessLUFS   *float64          `json:"loudness_lufs,omitempty"`
	OverdubCount   int               `json:"overdub_count,omitempty"`
	Kind           string            `json:"kind,omitempty"`
}

type OverdubVote struct {
	TrackID   uuid.UUID `json:"track_id"`
	UserID    uuid.UUID `json:"user_id"`
	OverdubID uuid.UUID `json:"overdub_id"`
	CreatedAt time.Time `json:"created_at"`
}

type Set struct {
	ID         uuid.UUID  `json:"id"`
	BandID     uuid.UUID  `json:"band_id"`
	Name       string     `json:"name"`
	SetType    string     `json:"set_type"`
	RecordedAt *time.Time `json:"recorded_at,omitempty"`
	Notes      string     `json:"notes,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	ItemCount  int        `json:"item_count,omitempty"`
	Items      []SetItem  `json:"items"`
	Tracks     []Track    `json:"tracks"`
}

type SetItem struct {
	ID         uuid.UUID  `json:"id"`
	SetID      uuid.UUID  `json:"set_id"`
	Position   int        `json:"position"`
	SongID     *uuid.UUID `json:"song_id,omitempty"`
	CustomName string     `json:"custom_name,omitempty"`
	StartMS    *int64     `json:"start_ms,omitempty"`
	EndMS      *int64     `json:"end_ms,omitempty"`
	Notes      string     `json:"notes,omitempty"`
	SongName   string     `json:"song_name,omitempty"`
}

type PerformItem struct {
	Position   int    `json:"position"`
	SongName   string `json:"song_name"`
	CustomName string `json:"custom_name"`
	Lyrics     string `json:"lyrics"`
	Tabs       string `json:"tabs"`
	Notes      string `json:"notes,omitempty"`
}

type SetTake struct {
	SetItemID    uuid.UUID       `json:"set_item_id"`
	SetID        uuid.UUID       `json:"set_id"`
	SetName      string          `json:"set_name"`
	SetType      string          `json:"set_type"`
	StartMS      int64           `json:"start_ms"`
	EndMS        int64           `json:"end_ms"`
	TrackID      uuid.UUID       `json:"track_id"`
	TrackTitle   string          `json:"track_title"`
	WaveformData json.RawMessage `json:"waveform_data,omitempty"`
	DurationMS   int64           `json:"duration_ms"`
	Format       string          `json:"format"`
	FileSize     int64           `json:"file_size"`
	Tags         []string        `json:"tags"`
	RecordedAt   *time.Time      `json:"recorded_at,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	Uploader     *User           `json:"uploader,omitempty"`
}

type ChatMessage struct {
	ID        uuid.UUID `json:"id"`
	BandID    uuid.UUID `json:"band_id"`
	UserID    uuid.UUID `json:"user_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	User      *User     `json:"user,omitempty"`
}

// Admin models

type AdminUser struct {
	ID          uuid.UUID `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	AvatarURL   string    `json:"avatar_url,omitempty"`
	IsAdmin     bool      `json:"is_admin"`
	IsDemo      bool      `json:"is_demo"`
	CreatedAt   time.Time `json:"created_at"`
	Bands       string    `json:"bands"`
}

type AdminBand struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	ColorScheme string    `json:"color_scheme"`
	CreatedAt   time.Time `json:"created_at"`
	MemberCount int       `json:"member_count"`
	TrackCount  int       `json:"track_count"`
}

type AdminInvite struct {
	ID            uuid.UUID  `json:"id"`
	Email         string     `json:"email"`
	Token         string     `json:"-"`
	ExpiresAt     time.Time  `json:"expires_at"`
	UsedBy        *uuid.UUID `json:"-"`
	UsedAt        *time.Time `json:"used_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	BandName      string     `json:"band_name"`
	CreatedByName string     `json:"created_by_name"`
	UsedByName    *string    `json:"used_by_name,omitempty"`
	Status        string     `json:"status"`
}

type Feedback struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	Body      string    `json:"body"`
	PageURL   string    `json:"page_url,omitempty"`
	ImagePath string    `json:"-"`
	HasImage  bool      `json:"has_image"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	User      *User     `json:"user,omitempty"`
}

type UnreadCount struct {
	BandID      uuid.UUID `json:"band_id"`
	BandSlug    string    `json:"band_slug"`
	NewTracks   int       `json:"new_tracks"`
	NewComments int       `json:"new_comments"`
	NewChats    int       `json:"new_chats"`
	NewSongs    int       `json:"new_songs"`
	Total       int       `json:"total"`
}

type ActivityItem struct {
	Type        string    `json:"type"`      // track, overdub, comment, song
	ActorName   string    `json:"actor_name"`
	Subject     string    `json:"subject"`
	BandSlug    string    `json:"band_slug"`
	BandName    string    `json:"band_name"`
	LinkID      string    `json:"link_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	// track/overdub only
	Peaks      []float64 `json:"peaks,omitempty"`
	DurationMs int64     `json:"duration_ms,omitempty"`
	// comment only
	Preview     string `json:"preview,omitempty"`
	TimestampMs *int64 `json:"timestamp_ms,omitempty"`
	// separate stream target (overdubs stream the overdub, not the parent)
	StreamID string `json:"stream_id,omitempty"`
	// unseen since last visit
	IsNew bool `json:"is_new"`
}

type APIToken struct {
	ID         uuid.UUID  `json:"id"`
	UserID     uuid.UUID  `json:"user_id"`
	BandID     uuid.UUID  `json:"band_id"`
	Token      string     `json:"-"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

type RppVersion struct {
	ID         uuid.UUID `json:"id"`
	TrackID    uuid.UUID `json:"track_id"`
	StorageKey string    `json:"-"`
	Version    int       `json:"version"`
	CreatedAt  time.Time `json:"created_at"`
}

type SyncFile struct {
	Filename  string     `json:"filename"`
	FileHash  string     `json:"file_hash"`
	OverdubID *uuid.UUID `json:"overdub_id,omitempty"`
}

type BakedStem struct {
	ReaperGUID string     `json:"reaper_guid"`
	ReaperName string     `json:"reaper_name"`
	RenderHash string     `json:"render_hash"`
	OverdubID  *uuid.UUID `json:"overdub_id,omitempty"`
}

type BandFile struct {
	ID          uuid.UUID `json:"id"`
	BandID      uuid.UUID `json:"band_id"`
	Name        string    `json:"name"`
	StorageKey  string    `json:"-"`
	FileSize    int64     `json:"file_size"`
	ContentType string    `json:"content_type"`
	UploadedBy  uuid.UUID `json:"uploaded_by"`
	CreatedAt   time.Time `json:"created_at"`
	Uploader    *User     `json:"uploader,omitempty"`
}

type Event struct {
	ID        int64           `json:"id"`
	SessionID string          `json:"session_id"`
	UserID    *uuid.UUID      `json:"user_id,omitempty"`
	Kind      string          `json:"kind"`
	Path      string          `json:"path"`
	Metadata  json.RawMessage `json:"metadata"`
	CreatedAt time.Time       `json:"created_at"`
	User      *User           `json:"user,omitempty"`
}

type EventSession struct {
	SessionID   string     `json:"session_id"`
	UserID      *uuid.UUID `json:"user_id,omitempty"`
	DisplayName string     `json:"display_name"`
	Email       string     `json:"email"`
	FirstSeen   time.Time  `json:"first_seen"`
	LastSeen    time.Time  `json:"last_seen"`
	EventCount  int        `json:"event_count"`
}

type Comment struct {
	ID          uuid.UUID  `json:"id"`
	TrackID     uuid.UUID  `json:"track_id"`
	UserID      uuid.UUID  `json:"user_id"`
	ParentID    *uuid.UUID `json:"parent_id,omitempty"`
	Body        string     `json:"body"`
	TimestampMS *int64     `json:"timestamp_ms,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	User        *User      `json:"user,omitempty"`
	Replies     []Comment  `json:"replies,omitempty"`
}
