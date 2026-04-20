package server

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"jerboa/internal/db"
	"jerboa/internal/models"
)

// mixSibling returns the path of the ephemeral session mixdown alongside the parent file.
func mixSibling(filePath string) string {
	if i := strings.LastIndex(filePath, "."); i >= 0 {
		return filePath[:i] + ".mix.opus"
	}
	return filePath + ".mix.opus"
}

const (
	mixDebounce   = 1500 * time.Millisecond
	mixMaxRunners = 2
)

type mixJob struct {
	timer   *time.Timer
	running bool
	pending bool
}

// MixBuilder coalesces rebuild requests for a track's ephemeral .mix.opus
// sibling and runs ffmpeg off the request path.
type MixBuilder struct {
	queries *db.Queries
	hub     *Hub
	sem     chan struct{}

	mu   sync.Mutex
	jobs map[uuid.UUID]*mixJob
}

func NewMixBuilder(queries *db.Queries, hub *Hub) *MixBuilder {
	sem := make(chan struct{}, mixMaxRunners)
	for i := 0; i < mixMaxRunners; i++ {
		sem <- struct{}{}
	}
	return &MixBuilder{
		queries: queries,
		hub:     hub,
		sem:     sem,
		jobs:    make(map[uuid.UUID]*mixJob),
	}
}

// Schedule requests a rebuild for parentID. Calls within the debounce window
// coalesce into one rebuild; calls during a run enqueue a single re-run.
func (b *MixBuilder) Schedule(parentID uuid.UUID) {
	b.mu.Lock()
	defer b.mu.Unlock()

	j, ok := b.jobs[parentID]
	if !ok {
		j = &mixJob{}
		b.jobs[parentID] = j
	}
	if j.running {
		j.pending = true
		return
	}
	if j.timer != nil {
		j.timer.Stop()
	}
	j.timer = time.AfterFunc(mixDebounce, func() { b.fire(parentID) })
}

func (b *MixBuilder) fire(parentID uuid.UUID) {
	b.mu.Lock()
	j := b.jobs[parentID]
	if j == nil {
		b.mu.Unlock()
		return
	}
	j.running = true
	j.pending = false
	j.timer = nil
	b.mu.Unlock()

	<-b.sem
	err := b.rebuild(parentID)
	b.sem <- struct{}{}
	if err != nil {
		slog.Error("mix: rebuild", "parent_id", parentID, "error", err)
	}

	b.mu.Lock()
	j.running = false
	pending := j.pending
	j.pending = false
	b.mu.Unlock()
	if pending {
		b.Schedule(parentID)
	}
}

func (b *MixBuilder) rebuild(parentID uuid.UUID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	parent, err := b.queries.GetTrack(ctx, parentID)
	if err != nil || parent == nil {
		return fmt.Errorf("get parent: %w", err)
	}
	overdubs, err := b.queries.ListOverdubs(ctx, parentID, uuid.Nil)
	if err != nil {
		return fmt.Errorf("list overdubs: %w", err)
	}

	mixPath := mixSibling(parent.FilePath)
	if len(overdubs) == 0 {
		// No overdubs — drop any stale mix file so Stream falls back to base opus.
		_ = os.Remove(mixPath)
		b.broadcast(parent)
		return nil
	}

	tmpDir, err := os.MkdirTemp("", "mix-*")
	if err != nil {
		return fmt.Errorf("mktemp: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	tmpOut := filepath.Join(tmpDir, "mix.opus")

	args, err := buildMixArgs(parent, overdubs, tmpOut)
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		slog.Warn("mix: ffmpeg failed", "parent_id", parentID, "err", err, "output", string(output))
		return err
	}

	if err := os.Rename(tmpOut, mixPath); err != nil {
		// Rename across filesystems can fail; fall back to copy.
		data, rerr := os.ReadFile(tmpOut)
		if rerr != nil {
			return rerr
		}
		if werr := os.WriteFile(mixPath, data, 0o644); werr != nil {
			return werr
		}
	}

	slog.Info("mix: rebuilt", "parent_id", parentID, "overdubs", len(overdubs), "path", mixPath)
	b.broadcast(parent)
	return nil
}

func (b *MixBuilder) broadcast(parent *models.Track) {
	if b.hub == nil {
		return
	}
	b.hub.Broadcast("band:"+parent.BandID.String(), WSMessage{
		Type:    "mix.ready",
		Payload: map[string]any{"track_id": parent.ID},
	})
}

// buildMixArgs constructs ffmpeg args to mix parent + overdubs into an Opus file.
// Same filter shape as doBounceMix but writes opus directly, no DB work.
func buildMixArgs(parent *models.Track, overdubs []models.Track, outPath string) ([]string, error) {
	var minOffset int64
	for _, od := range overdubs {
		if od.OffsetMS < minOffset {
			minOffset = od.OffsetMS
		}
	}
	parentDelay := int64(0)
	if minOffset < 0 {
		parentDelay = -minOffset
	}

	args := []string{"-i", parent.FilePath}
	for _, od := range overdubs {
		args = append(args, "-i", od.FilePath)
	}

	numInputs := 1 + len(overdubs)
	var filterParts []string
	var mixLabels []string

	if parentDelay > 0 {
		filterParts = append(filterParts, fmt.Sprintf("[0]adelay=%d|%d[p]", parentDelay, parentDelay))
		mixLabels = append(mixLabels, "[p]")
	} else {
		mixLabels = append(mixLabels, "[0]")
	}

	for i, od := range overdubs {
		ffIdx := i + 1
		delay := parentDelay + od.OffsetMS
		if delay > 0 {
			label := fmt.Sprintf("[d%d]", i)
			filterParts = append(filterParts, fmt.Sprintf("[%d]adelay=%d|%d%s", ffIdx, delay, delay, label))
			mixLabels = append(mixLabels, label)
		} else {
			mixLabels = append(mixLabels, fmt.Sprintf("[%d]", ffIdx))
		}
	}

	mixFilter := strings.Join(mixLabels, "") + fmt.Sprintf("amix=inputs=%d:duration=longest:normalize=0", numInputs)
	filterParts = append(filterParts, mixFilter)
	filter := strings.Join(filterParts, ";")

	args = append(args,
		"-filter_complex", filter,
		"-ac", "2",
		"-ar", "48000",
		"-c:a", "libopus",
		"-b:a", "96k",
		"-vbr", "on",
		"-y", outPath,
	)
	return args, nil
}
