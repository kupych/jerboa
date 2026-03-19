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
	CreatedAt   time.Time `json:"created_at"`
}

type Band struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedBy uuid.UUID `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
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
	CreatedAt    time.Time       `json:"created_at"`
	Uploader     *User           `json:"uploader,omitempty"`
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
