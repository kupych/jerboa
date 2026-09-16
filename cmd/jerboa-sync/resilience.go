package main

// Pieces that keep a long unattended sync alive: a hash cache so we don't
// re-read the whole project on every run, a stall watchdog so a dead
// connection can't hang overnight, retry backoff, and keeping the Mac awake.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync/atomic"
	"time"
)

// A transfer with no bytes moving for this long is treated as dead and retried
// rather than left hanging.
const stallTimeout = 3 * time.Minute

// retryDelays is deliberately long-tailed: an unattended overnight run should
// ride out a router reboot or a brief ISP outage rather than give up.
var retryDelays = []time.Duration{5 * time.Second, 15 * time.Second, 45 * time.Second, 2 * time.Minute, 5 * time.Minute}

// ---------------------------------------------------------------- hash cache

// hashEntry records what a file looked like when we last hashed it. A file
// whose size and mtime are unchanged is assumed to have the same content —
// the same bet rsync makes by default.
type hashEntry struct {
	Size      int64  `json:"size"`
	ModTimeNs int64  `json:"mtime_ns"`
	Hash      string `json:"hash"`
}

type hashCache struct {
	path    string
	entries map[string]hashEntry
	dirty   bool
}

// loadHashCache keeps its state in the user cache dir, keyed by project path,
// so it never litters the .band package or the music folder.
func loadHashCache(projectPath string) *hashCache {
	c := &hashCache{entries: map[string]hashEntry{}}

	dir, err := os.UserCacheDir()
	if err != nil {
		return c
	}
	sum := sha256.Sum256([]byte(projectPath))
	c.path = filepath.Join(dir, "jerboa-sync", hex.EncodeToString(sum[:8])+".json")

	if data, err := os.ReadFile(c.path); err == nil {
		json.Unmarshal(data, &c.entries)
	}
	return c
}

func (c *hashCache) save() {
	if !c.dirty || c.path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0700); err != nil {
		return
	}
	if data, err := json.Marshal(c.entries); err == nil {
		os.WriteFile(c.path, data, 0600)
	}
}

// hash returns the file's SHA-256, reusing the cached value when size and
// mtime are untouched. On a 12 GB project this turns a multi-minute read of
// every byte into a directory walk.
func (c *hashCache) hash(f localFile) (string, error) {
	info, err := os.Stat(f.Abs)
	if err != nil {
		return "", err
	}
	if e, ok := c.entries[f.Rel]; ok && e.Size == info.Size() && e.ModTimeNs == info.ModTime().UnixNano() {
		return e.Hash, nil
	}

	h, err := hashFile(f.Abs)
	if err != nil {
		return "", err
	}
	c.entries[f.Rel] = hashEntry{Size: info.Size(), ModTimeNs: info.ModTime().UnixNano(), Hash: h}
	c.dirty = true
	return h, nil
}

// forget drops an entry so the next run re-hashes it.
func (c *hashCache) forget(rel string) {
	delete(c.entries, rel)
	c.dirty = true
}

// ---------------------------------------------------------------- watchdog

// stallWatchdog cancels ctx when a transfer stops moving bytes. Without it a
// half-open TCP connection — the normal result of a laptop's wifi dropping —
// leaves the upload blocked forever instead of retrying.
type stallWatchdog struct {
	moved  atomic.Int64
	cancel context.CancelFunc
	done   chan struct{}
}

func newStallWatchdog(ctx context.Context) (context.Context, *stallWatchdog) {
	ctx, cancel := context.WithCancel(ctx)
	w := &stallWatchdog{cancel: cancel, done: make(chan struct{})}

	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		last := int64(0)
		idle := time.Duration(0)
		for {
			select {
			case <-w.done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				now := w.moved.Load()
				if now != last {
					last, idle = now, 0
					continue
				}
				if idle += 15 * time.Second; idle >= stallTimeout {
					cancel()
					return
				}
			}
		}
	}()
	return ctx, w
}

func (w *stallWatchdog) progress(n int64) { w.moved.Add(n) }

func (w *stallWatchdog) stop() {
	close(w.done)
	w.cancel()
}

// ---------------------------------------------------------------- retry

// withRetry runs op until it succeeds or the attempts run out, reporting each
// failure so an overnight log shows what happened. op receives the attempt
// number, 0 for the first try, so callers can do extra work only on a retry.
func withRetry(label string, op func(attempt int) error) error {
	err := op(0)
	for i, delay := range retryDelays {
		if err == nil {
			return nil
		}
		fmt.Printf("\r  %s  FAILED: %v — retry %d/%d in %s\n", label, err, i+1, len(retryDelays), delay)
		time.Sleep(delay)
		err = op(i + 1)
	}
	return err
}

// transferError strips the query string off a failed request's URL. Presigned
// URLs carry a signature that grants access to the object, so they don't
// belong in a console log the user may paste somewhere.
func transferError(err error) error {
	var uerr *url.Error
	if !errors.As(err, &uerr) {
		return err
	}
	shown := uerr.URL
	if u, perr := url.Parse(uerr.URL); perr == nil {
		u.RawQuery = ""
		shown = u.Redacted()
	}
	return fmt.Errorf("%s %s: %w", uerr.Op, shown, uerr.Err)
}

// ---------------------------------------------------------------- sleep

// keepAwake stops macOS idle-sleeping mid-transfer. caffeinate exits with us,
// so there's nothing to clean up.
func keepAwake() {
	if runtime.GOOS != "darwin" {
		return
	}
	cmd := exec.Command("caffeinate", "-i", "-w", strconv.Itoa(os.Getpid()))
	if err := cmd.Start(); err == nil {
		go cmd.Wait() // reap it rather than leave a zombie
	}
}
