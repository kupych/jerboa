package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ServerURL string `json:"server_url"`
	BandSlug  string `json:"band_slug"`
	Token     string `json:"token"`
	SongID    string `json:"song_id,omitempty"`
}

type syncState struct {
	TrackID    string       `json:"track_id"`
	Files      []fileInfo   `json:"files"`
	BakedStems []bakedStem  `json:"baked_stems"`
}

type fileInfo struct {
	Filename  string  `json:"filename"`
	FileHash  string  `json:"file_hash"`
	OverdubID *string `json:"overdub_id,omitempty"`
}

type bakedStem struct {
	ReaperGUID string  `json:"reaper_guid"`
	ReaperName string  `json:"reaper_name"`
	RenderHash string  `json:"render_hash"`
	OverdubID  *string `json:"overdub_id,omitempty"`
}

type sessionInfo struct {
	TrackID     string `json:"track_id"`
	SessionName string `json:"session_name"`
}

func main() {
	manifestMode := false
	for _, a := range os.Args[1:] {
		if a == "--manifest" {
			manifestMode = true
		}
	}

	cfg, err := loadConfig()
	if err != nil {
		fatalf("config: %v\n\nMake sure you downloaded this utility from Jerboa — it needs to be pre-configured.", err)
	}

	client := &http.Client{Timeout: 10 * time.Minute}

	if manifestMode {
		rppPath, err := findRPP(".")
		if err != nil {
			fatalf("manifest: %v", err)
		}
		sessionName := strings.TrimSuffix(filepath.Base(rppPath), filepath.Ext(rppPath))
		if err := emitManifest(client, cfg, rppPath, sessionName); err != nil {
			fatalf("manifest: %v", err)
		}
		return
	}

	fmt.Printf("jerboa-sync  →  %s / %s\n\n", cfg.ServerURL, cfg.BandSlug)

	rppPath, err := findRPP(".")
	if err != nil {
		// No .rpp found — interactive pull mode
		runPull(client, cfg, "")
		return
	}

	fmt.Printf("project : %s\n", filepath.Base(rppPath))
	sessionName := strings.TrimSuffix(filepath.Base(rppPath), filepath.Ext(rppPath))

	runPush(client, cfg, rppPath, sessionName)
	runBake(client, cfg, rppPath, sessionName)
	runPullInto(client, cfg, sessionName, filepath.Dir(rppPath))
}

// emitManifest writes one TSV line to stdout per Reaper track whose render
// hash differs from the server's known baked stem. Format:
//
//	# session_name=<name>
//	# rpp_path=<abs path>
//	# track_id=<uuid>
//	# bake_dir=<abs path>
//	<reaper_guid>\t<reaper_name>\t<render_hash>
//
// Used by the Lua driver inside Reaper to drive the per-track bake step.
func emitManifest(client *http.Client, cfg *Config, rppPath, sessionName string) error {
	tracks, err := ParseRPPTracks(rppPath)
	if err != nil {
		return fmt.Errorf("parse rpp: %w", err)
	}
	state, err := getState(client, cfg, sessionName)
	if err != nil {
		return fmt.Errorf("get state: %w", err)
	}

	serverHash := map[string]string{}
	for _, s := range state.BakedStems {
		serverHash[s.ReaperGUID] = s.RenderHash
	}

	absRPP, _ := filepath.Abs(rppPath)
	bakeDir := filepath.Join(filepath.Dir(absRPP), ".jerboa-bake")

	fmt.Printf("# session_name=%s\n", sessionName)
	fmt.Printf("# rpp_path=%s\n", absRPP)
	fmt.Printf("# track_id=%s\n", state.TrackID)
	fmt.Printf("# bake_dir=%s\n", bakeDir)

	for _, t := range tracks {
		if t.GUID == "" {
			continue
		}
		hash, err := renderHash(t)
		if err != nil {
			continue
		}
		known, present := serverHash[t.GUID]
		var status string
		switch {
		case !present:
			status = "unbaked"
		case known == hash:
			status = "clean"
		default:
			status = "stale"
		}
		fmt.Printf("%s\t%s\t%s\t%s\n", t.GUID, t.Name, hash, status)
	}
	return nil
}

// runPush uploads local audio files and .rpp to the server.
func runPush(client *http.Client, cfg *Config, rppPath, sessionName string) {
	items, err := ParseRPP(rppPath)
	if err != nil {
		fatalf("parse rpp: %v", err)
	}
	fmt.Printf("items   : %d audio file(s) found\n\n", len(items))

	state, err := getState(client, cfg, sessionName)
	if err != nil {
		fatalf("get state: %v", err)
	}

	serverHashes := map[string]string{}
	for _, f := range state.Files {
		serverHashes[f.Filename] = f.FileHash
	}

	uploaded, skipped, failed := 0, 0, 0

	for _, item := range items {
		base := filepath.Base(item.FilePath)

		if _, err := os.Stat(item.FilePath); os.IsNotExist(err) {
			fmt.Printf("  skip  %s (not found on disk)\n", base)
			skipped++
			continue
		}

		hash, err := hashFile(item.FilePath)
		if err != nil {
			fmt.Printf("  error %s: %v\n", base, err)
			failed++
			continue
		}

		if serverHashes[base] == hash {
			fmt.Printf("  ok    %s\n", base)
			skipped++
			continue
		}

		fmt.Printf("  up    %s  offset=%dms ... ", base, item.OffsetMS)
		if err := uploadAudio(client, cfg, state.TrackID, item, hash); err != nil {
			fmt.Printf("FAILED: %v\n", err)
			failed++
		} else {
			fmt.Printf("done\n")
			uploaded++
		}
	}

	rppHash, _ := hashFile(rppPath)
	fmt.Printf("\n  rpp   %s ... ", filepath.Base(rppPath))
	if err := uploadRPP(client, cfg, state.TrackID, rppPath); err != nil {
		fmt.Printf("FAILED: %v\n", err)
	} else {
		fmt.Printf("done\n")
		_ = rppHash
	}

	fmt.Printf("\npush — %d uploaded, %d skipped, %d failed\n\n", uploaded, skipped, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

// runBake computes per-track render hashes, diffs them against the server's
// known baked stems, and uploads any matching WAVs found in .jerboa-bake/.
// It does NOT invoke Reaper itself — the GUI/script step does that — but it
// always reports which tracks are stale so an external bake step can act.
func runBake(client *http.Client, cfg *Config, rppPath, sessionName string) {
	tracks, err := ParseRPPTracks(rppPath)
	if err != nil {
		fmt.Printf("\nbake — skipped: parse rpp tracks: %v\n", err)
		return
	}
	if len(tracks) == 0 {
		return
	}

	state, err := getState(client, cfg, sessionName)
	if err != nil {
		fmt.Printf("\nbake — skipped: get state: %v\n", err)
		return
	}

	serverHash := map[string]string{}
	for _, s := range state.BakedStems {
		serverHash[s.ReaperGUID] = s.RenderHash
	}

	bakeDir := filepath.Join(filepath.Dir(rppPath), ".jerboa-bake")

	fmt.Printf("\nbake — %d track(s)\n", len(tracks))
	staleNoFile := 0
	uploaded, skipped, failed := 0, 0, 0

	for _, t := range tracks {
		if t.GUID == "" {
			continue // track has no TRACKID line; skip silently
		}
		hash, err := renderHash(t)
		if err != nil {
			fmt.Printf("  error %s: hash: %v\n", t.Name, err)
			failed++
			continue
		}
		label := t.Name
		if label == "" {
			label = t.GUID[:8]
		}
		if serverHash[t.GUID] == hash {
			fmt.Printf("  ok    %s\n", label)
			skipped++
			continue
		}

		// Stale or missing — look for a baked WAV.
		wavPath := filepath.Join(bakeDir, t.GUID+".wav")
		if _, err := os.Stat(wavPath); err != nil {
			fmt.Printf("  need  %s  (no %s)\n", label, filepath.Join(".jerboa-bake", t.GUID+".wav"))
			staleNoFile++
			continue
		}

		fmt.Printf("  up    %s ... ", label)
		if err := uploadBaked(client, cfg, state.TrackID, t.GUID, t.Name, hash, wavPath); err != nil {
			fmt.Printf("FAILED: %v\n", err)
			failed++
		} else {
			fmt.Printf("done\n")
			uploaded++
		}
	}

	fmt.Printf("\nbake — %d uploaded, %d up-to-date, %d stale (need render), %d failed\n", uploaded, skipped, staleNoFile, failed)
}

// renderHash hashes the verbatim <TRACK> block plus the sorted SHA-256 hashes
// of every source media file referenced inside it. Stable across cosmetic
// re-saves and changes the moment FX, automation, fader, or media changes.
func renderHash(t RPPTrack) (string, error) {
	h := sha256.New()
	h.Write(t.BlockBytes)

	var mediaHashes []string
	for _, item := range t.Items {
		mh, err := hashFile(item.FilePath)
		if err != nil {
			if os.IsNotExist(err) {
				continue // missing media — render hash still meaningful from block bytes
			}
			return "", err
		}
		mediaHashes = append(mediaHashes, mh)
	}
	sort.Strings(mediaHashes)
	for _, mh := range mediaHashes {
		h.Write([]byte(mh))
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func uploadBaked(client *http.Client, cfg *Config, trackID, reaperGUID, reaperName, renderHash, wavPath string) error {
	f, err := os.Open(wavPath)
	if err != nil {
		return err
	}
	defer f.Close()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("track_id", trackID)
	mw.WriteField("reaper_guid", reaperGUID)
	mw.WriteField("reaper_name", reaperName)
	mw.WriteField("render_hash", renderHash)

	fw, err := mw.CreateFormFile("file", filepath.Base(wavPath))
	if err != nil {
		return err
	}
	if _, err := io.Copy(fw, f); err != nil {
		return err
	}
	mw.Close()

	url := fmt.Sprintf("%s/api/bands/%s/sync/baked", cfg.ServerURL, cfg.BandSlug)
	req, _ := http.NewRequest("POST", url, &buf)
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%d: %s", resp.StatusCode, body)
	}
	return nil
}

// runPull is the interactive no-.rpp mode: list sessions, prompt, pull.
func runPull(client *http.Client, cfg *Config, sessionName string) {
	sessions, err := listSessions(client, cfg)
	if err != nil {
		fatalf("list sessions: %v", err)
	}
	if len(sessions) == 0 {
		fmt.Println("No sessions found on server.")
		waitIfWindows()
		return
	}

	var chosen sessionInfo
	if sessionName != "" {
		for _, s := range sessions {
			if s.SessionName == sessionName {
				chosen = s
				break
			}
		}
		if chosen.TrackID == "" {
			fatalf("session %q not found on server", sessionName)
		}
	} else {
		fmt.Println("Available sessions:")
		for i, s := range sessions {
			fmt.Printf("  %d) %s\n", i+1, s.SessionName)
		}
		fmt.Print("\nEnter number to pull: ")
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		n, err := strconv.Atoi(strings.TrimSpace(scanner.Text()))
		if err != nil || n < 1 || n > len(sessions) {
			fatalf("invalid selection")
		}
		chosen = sessions[n-1]
	}

	fmt.Printf("\npulling %s ...\n\n", chosen.SessionName)
	runPullInto(client, cfg, chosen.SessionName, ".")
}

// runPullInto downloads any server-side files missing or conflicting locally.
func runPullInto(client *http.Client, cfg *Config, sessionName, dir string) {
	state, err := getState(client, cfg, sessionName)
	if err != nil {
		fatalf("get state: %v", err)
	}

	// Pull .rpp first (always — we need it to find correct file paths)
	rppLocal := filepath.Join(dir, sessionName+".rpp")
	rppExists := true
	if _, err := os.Stat(rppLocal); os.IsNotExist(err) {
		rppExists = false
		fmt.Printf("  rpp   %s.rpp ... ", sessionName)
		if err := downloadRPP(client, cfg, state.TrackID, rppLocal); err != nil {
			fmt.Printf("FAILED: %v\n", err)
		} else {
			fmt.Printf("done\n")
		}
	}
	_ = rppExists

	// Parse .rpp to build basename → relative path index
	rppPaths := map[string]string{} // basename → path relative to dir
	if items, err := ParseRPP(rppLocal); err == nil {
		for _, item := range items {
			rel, err := filepath.Rel(dir, item.FilePath)
			if err != nil {
				rel = filepath.Base(item.FilePath)
			}
			rppPaths[filepath.Base(item.FilePath)] = rel
		}
	}

	downloaded, skipped, conflicts := 0, 0, 0

	for _, f := range state.Files {
		if f.OverdubID == nil || *f.OverdubID == "" {
			continue
		}

		// Use the path the .rpp expects, falling back to flat in dir
		relPath, ok := rppPaths[f.Filename]
		if !ok {
			relPath = f.Filename
		}
		localPath := filepath.Join(dir, relPath)
		dest := localPath

		if _, err := os.Stat(localPath); err == nil {
			// File exists — check hash
			localHash, err := hashFile(localPath)
			if err == nil && localHash == f.FileHash {
				fmt.Printf("  ok    %s\n", relPath)
				skipped++
				continue
			}
			// Conflict: same name, different content
			ext := filepath.Ext(localPath)
			base := strings.TrimSuffix(localPath, ext)
			dest = base + "_remote" + ext
			fmt.Printf("  conf  %s → %s\n", relPath, filepath.Base(dest))
			conflicts++
		}

		fmt.Printf("  dl    %s ... ", relPath)
		if err := downloadAudio(client, cfg, *f.OverdubID, dest); err != nil {
			fmt.Printf("FAILED: %v\n", err)
		} else {
			fmt.Printf("done\n")
			downloaded++
		}
	}

	fmt.Printf("\npull — %d downloaded, %d skipped, %d conflict(s) saved as _remote\n", downloaded, skipped, conflicts)
	waitIfWindows()
}

func findRPP(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var found []string
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".rpp") {
			found = append(found, filepath.Join(dir, e.Name()))
		}
	}
	switch len(found) {
	case 0:
		return "", fmt.Errorf("no .rpp file found in %s", dir)
	case 1:
		return found[0], nil
	default:
		return "", fmt.Errorf("multiple .rpp files found — run from a single-project directory")
	}
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func listSessions(client *http.Client, cfg *Config) ([]sessionInfo, error) {
	url := fmt.Sprintf("%s/api/bands/%s/sync/sessions", cfg.ServerURL, cfg.BandSlug)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+cfg.Token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Sessions []sessionInfo `json:"sessions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Sessions, nil
}

func getState(client *http.Client, cfg *Config, sessionName string) (*syncState, error) {
	url := fmt.Sprintf("%s/api/bands/%s/sync/state/%s", cfg.ServerURL, cfg.BandSlug, sessionName)
	if cfg.SongID != "" {
		url += "?song_id=" + cfg.SongID
	}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+cfg.Token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, body)
	}

	var state syncState
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return nil, err
	}
	return &state, nil
}

func uploadAudio(client *http.Client, cfg *Config, trackID string, item RPPItem, hash string) error {
	f, err := os.Open(item.FilePath)
	if err != nil {
		return err
	}
	defer f.Close()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("track_id", trackID)
	mw.WriteField("filename", filepath.Base(item.FilePath))
	mw.WriteField("file_hash", hash)
	mw.WriteField("offset_ms", fmt.Sprintf("%d", item.OffsetMS))
	if item.Gain > 0 {
		mw.WriteField("gain", fmt.Sprintf("%.6f", item.Gain))
	}

	fw, err := mw.CreateFormFile("file", filepath.Base(item.FilePath))
	if err != nil {
		return err
	}
	if _, err := io.Copy(fw, f); err != nil {
		return err
	}
	mw.Close()

	url := fmt.Sprintf("%s/api/bands/%s/sync/file", cfg.ServerURL, cfg.BandSlug)
	req, _ := http.NewRequest("POST", url, &buf)
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%d: %s", resp.StatusCode, body)
	}
	return nil
}

func uploadRPP(client *http.Client, cfg *Config, trackID string, rppPath string) error {
	f, err := os.Open(rppPath)
	if err != nil {
		return err
	}
	defer f.Close()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("track_id", trackID)
	fw, err := mw.CreateFormFile("file", filepath.Base(rppPath))
	if err != nil {
		return err
	}
	if _, err := io.Copy(fw, f); err != nil {
		return err
	}
	mw.Close()

	url := fmt.Sprintf("%s/api/bands/%s/sync/rpp", cfg.ServerURL, cfg.BandSlug)
	req, _ := http.NewRequest("POST", url, &buf)
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%d: %s", resp.StatusCode, body)
	}
	return nil
}

func downloadRPP(client *http.Client, cfg *Config, trackID, dest string) error {
	url := fmt.Sprintf("%s/api/bands/%s/sync/rpp/%s", cfg.ServerURL, cfg.BandSlug, trackID)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+cfg.Token)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%d: %s", resp.StatusCode, body)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func downloadAudio(client *http.Client, cfg *Config, overdubID, dest string) error {
	url := fmt.Sprintf("%s/api/bands/%s/tracks/%s/stream?dl=1", cfg.ServerURL, cfg.BandSlug, overdubID)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+cfg.Token)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%d: %s", resp.StatusCode, body)
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// loadConfig reads the embedded config from the end of the binary.
// Format: [binary][JSON config][uint64 LE length of JSON]
func loadConfig() (*Config, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}

	f, err := os.Open(exe)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	size, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, err
	}
	if size < 8 {
		return nil, fmt.Errorf("no embedded config found")
	}

	// Read 8-byte length from end
	if _, err := f.Seek(-8, io.SeekEnd); err != nil {
		return nil, err
	}
	var cfgLen uint64
	if err := binary.Read(f, binary.LittleEndian, &cfgLen); err != nil {
		return nil, err
	}

	if cfgLen == 0 || int64(cfgLen) > size-8 {
		return nil, fmt.Errorf("no embedded config found")
	}

	// Read JSON config
	if _, err := f.Seek(-(int64(cfgLen) + 8), io.SeekEnd); err != nil {
		return nil, err
	}
	cfgData := make([]byte, cfgLen)
	if _, err := io.ReadFull(f, cfgData); err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(cfgData, &cfg); err != nil {
		return nil, fmt.Errorf("corrupt config: %w", err)
	}
	if cfg.ServerURL == "" || cfg.Token == "" {
		return nil, fmt.Errorf("incomplete config")
	}
	return &cfg, nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	waitIfWindows()
	os.Exit(1)
}
