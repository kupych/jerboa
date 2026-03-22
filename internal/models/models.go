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

type Song struct {
	ID        uuid.UUID `json:"id"`
	BandID    uuid.UUID `json:"band_id"`
	Name      string    `json:"name"`
	Lyrics    string    `json:"lyrics"`
	Tabs      string    `json:"tabs"`
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
	CreatedAt    time.Time       `json:"created_at"`
	Uploader     *User             `json:"uploader,omitempty"`
	Song         *Song             `json:"song,omitempty"`
	Personnel    []TrackPersonnel  `json:"personnel,omitempty"`
	Overdubs     []Track           `json:"overdubs,omitempty"`
	VoteCount    int               `json:"vote_count,omitempty"`
	UserVoted    bool              `json:"user_voted,omitempty"`
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
