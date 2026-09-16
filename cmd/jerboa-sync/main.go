package main

import (
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
	TrackID    string      `json:"track_id"`
	Files      []fileInfo  `json:"files"`
	BakedStems []bakedStem `json:"baked_stems"`
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

const usage = `usage: jerboa-sync [flags] [project.band | folder ...]
       jerboa-sync pair --server URL
       jerboa-sync status
       jerboa-sync forget PATH
       jerboa-sync delete PROJECT

Backs up GarageBand projects to your band, file by file, and syncs Reaper
sessions when run inside a Reaper project folder.

Projects you name — on the command line, by dropping them on the Jerboa Sync
app, or with Finder's "Sync to Jerboa" — are remembered. Run it with no
arguments to sync everything it remembers.

commands:
  pair --server URL   connect this computer to a band (the installer does this)
  status              check the connection; exits non-zero if it isn't working
  forget PATH         stop syncing a remembered project or folder
  delete PROJECT      delete a project's backup for the whole band (admins only;
                      never touches anyone's local copy)

flags:
  --pull              pick a session/project from the server and download it
  --dry-run           GarageBand: show what would be sent/received, change nothing
  --list              GarageBand: list every file, not just a per-track summary
  --exclude PATTERN   GarageBand: skip files/folders matching PATTERN (repeatable);
                      the same patterns can go one per line in .jerboaignore
                      next to the .band
`

func main() {
	stored = loadStored()
	args := os.Args[1:]

	// Subcommands come first so they can't be mistaken for project paths.
	if len(args) > 0 {
		switch args[0] {
		case "pair":
			server := ""
			for i := 1; i < len(args); i++ {
				switch {
				case args[i] == "--server" && i+1 < len(args):
					i++
					server = args[i]
				case strings.HasPrefix(args[i], "--server="):
					server = strings.TrimPrefix(args[i], "--server=")
				}
			}
			if server == "" {
				server = stored.ServerURL // re-pairing with the same server
			}
			if err := runPair(server); err != nil {
				fatalf("%v", err)
			}
			return
		case "status":
			os.Exit(runStatus())
		case "delete":
			if len(args) != 2 {
				fatalf("usage: jerboa-sync delete PROJECT")
			}
			cfg, err := resolveConfig()
			if err != nil {
				fatalf("%v", err)
			}
			finish(runDelete(cfg, args[1]))
		case "forget":
			if len(args) < 2 {
				fatalf("usage: jerboa-sync forget PATH")
			}
			for _, p := range args[1:] {
				if forgetProject(p) {
					fmt.Printf("forgot %s\n", p)
				} else {
					fmt.Printf("%s wasn't remembered\n", p)
				}
			}
			return
		}
	}

	manifestMode := false
	pullMode := false
	var opts mirrorOpts
	var targets []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--manifest":
			manifestMode = true
		case a == "--pull":
			pullMode = true
		case a == "--dry-run":
			opts.DryRun = true
		case a == "--list":
			opts.Verbose = true
		case a == "--exclude" && i+1 < len(args):
			i++
			opts.Excludes = append(opts.Excludes, args[i])
		case strings.HasPrefix(a, "--exclude="):
			opts.Excludes = append(opts.Excludes, strings.TrimPrefix(a, "--exclude="))
		case a == "-h" || a == "--help":
			fmt.Print(usage)
			return
		case !strings.HasPrefix(a, "-"):
			targets = append(targets, a)
		default:
			fmt.Fprint(os.Stderr, usage)
			fatalf("unknown argument %q", a)
		}
	}

	cfg, err := resolveConfig()
	if err != nil {
		fatalf("%v.\n\nRun the installer from your band's settings page on Jerboa, or:\n  jerboa-sync pair --server https://your-jerboa-server", err)
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

	if pullMode {
		finish(runPull(client, cfg, opts))
	}
	if len(targets) > 0 {
		finish(syncTargets(cfg, targets, opts))
	}

	if rppPath, err := findRPP("."); err == nil {
		fmt.Printf("project : %s\n", filepath.Base(rppPath))
		sessionName := strings.TrimSuffix(filepath.Base(rppPath), filepath.Ext(rppPath))
		runPush(client, cfg, rppPath, sessionName)
		runBake(client, cfg, rppPath, sessionName)
		runPullInto(client, cfg, sessionName, filepath.Dir(rppPath))
		finish(0)
	}

	finish(runInteractive(client, cfg, opts))
}

// finish is the single exit point for a normal run, so a double-clicked
// window on Windows pauses exactly once before closing.
func finish(code int) {
	waitIfWindows()
	os.Exit(code)
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

	return postMultipart(client, cfg, "sync/baked", filepath.Base(wavPath), f, [][2]string{
		{"track_id", trackID},
		{"reaper_guid", reaperGUID},
		{"reaper_name", reaperName},
		{"render_hash", renderHash},
	})
}

// runPull is the interactive pull mode: list Reaper sessions and GarageBand
// projects on the server, prompt, pull.
func runPull(client *http.Client, cfg *Config, opts mirrorOpts) int {
	sessions, err := listSessions(client, cfg)
	if err != nil {
		fatalf("list sessions: %v", err)
	}
	projects, err := listMirrorProjects(cfg)
	if err != nil {
		fatalf("list GarageBand projects: %v", err)
	}
	if len(sessions)+len(projects) == 0 {
		fmt.Println("No sessions found on server.")
		return 0
	}

	fmt.Println("Available sessions:")
	for i, s := range sessions {
		fmt.Printf("  %d) %s  (Reaper)\n", i+1, s.SessionName)
	}
	for i, p := range projects {
		fmt.Printf("  %d) %s  (GarageBand, %d files, %s)\n", len(sessions)+i+1, p.Name, p.FileCount, humanSize(p.TotalSize))
	}
	n := promptChoice(len(sessions) + len(projects))

	if n > len(sessions) {
		dest := "."
		if dir := garageBandDir(); dir != "" && len(findBands(".")) == 0 {
			dest = dir
		}
		fmt.Println()
		return runBandPull(cfg, projects[n-len(sessions)-1].Name, dest, opts)
	}

	chosen := sessions[n-1]
	fmt.Printf("\npulling %s ...\n\n", chosen.SessionName)
	runPullInto(client, cfg, chosen.SessionName, ".")
	return 0
}

func promptChoice(max int) int {
	n, err := strconv.Atoi(readLine("\nEnter number: "))
	if err != nil || n < 1 || n > max {
		fatalf("invalid selection")
	}
	return n
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

// streamMultipart builds a multipart body that is generated as the request is
// sent, so a file is never held in memory. The previous version copied whole
// files into a bytes.Buffer, which on a 4 GB laptop meant a 500 MB stem cost
// 500 MB of RAM before a single byte went out.
//
// The file part goes first, as streamMultipartToStorage on the server expects.
func streamMultipart(filename string, file io.Reader, fields [][2]string) (*io.PipeReader, string) {
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)

	go func() {
		var err error
		defer func() { pw.CloseWithError(err) }()

		var fw io.Writer
		if fw, err = mw.CreateFormFile("file", filename); err != nil {
			return
		}
		if _, err = io.Copy(fw, file); err != nil {
			return
		}
		for _, kv := range fields {
			if err = mw.WriteField(kv[0], kv[1]); err != nil {
				return
			}
		}
		err = mw.Close()
	}()

	return pr, mw.FormDataContentType()
}

// postMultipart sends one streamed upload and expects 201.
func postMultipart(client *http.Client, cfg *Config, endpoint, filename string, file io.Reader, fields [][2]string) error {
	body, contentType := streamMultipart(filename, file, fields)
	defer body.Close()

	url := fmt.Sprintf("%s/api/bands/%s/%s", cfg.ServerURL, cfg.BandSlug, endpoint)
	req, _ := http.NewRequest("POST", url, body)
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("Content-Type", contentType)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}

func uploadAudio(client *http.Client, cfg *Config, trackID string, item RPPItem, hash string) error {
	f, err := os.Open(item.FilePath)
	if err != nil {
		return err
	}
	defer f.Close()

	base := filepath.Base(item.FilePath)
	fields := [][2]string{
		{"track_id", trackID},
		{"filename", base},
		{"file_hash", hash},
		{"offset_ms", fmt.Sprintf("%d", item.OffsetMS)},
	}
	if item.Gain > 0 {
		fields = append(fields, [2]string{"gain", fmt.Sprintf("%.6f", item.Gain)})
	}
	return postMultipart(client, cfg, "sync/file", base, f, fields)
}

func uploadRPP(client *http.Client, cfg *Config, trackID string, rppPath string) error {
	f, err := os.Open(rppPath)
	if err != nil {
		return err
	}
	defer f.Close()

	return postMultipart(client, cfg, "sync/rpp", filepath.Base(rppPath), f, [][2]string{{"track_id", trackID}})
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

// loadEmbeddedConfig reads the config appended to binaries downloaded from the band settings page.
// Format: [binary][JSON config][uint64 LE length of JSON]
func loadEmbeddedConfig() (*Config, error) {
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
