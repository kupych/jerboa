package main

// GarageBand projects are .band packages (folders) whose ProjectData is a
// closed binary format, so unlike .rpp we can't tell which audio is used or
// muted. Instead we mirror the package file-by-file, rsync-style: skip what
// the server already has, never send stuff GarageBand regenerates, and let the
// user exclude more with --exclude or a .jerboaignore file next to the .band.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

type mirrorOpts struct {
	DryRun   bool
	Verbose  bool
	Excludes []string
}

type mirrorFile struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
	Size int64  `json:"size"`
}

type mirrorProject struct {
	Name      string `json:"name"`
	FileCount int    `json:"file_count"`
	TotalSize int64  `json:"total_size"`
}

type localFile struct {
	Rel  string // slash-separated, relative to the package root
	Abs  string
	Size int64
}

// Always skipped. Matched case-insensitively against any path component.
var defaultExcludes = []string{
	"*.nosync", // Undo Data.nosync, Freeze Files.nosync — undo history and frozen-track renders
	"Output",   // mixdown preview GarageBand writes for the Media Browser
	".DS_Store",
}

const ignoreFileName = ".jerboaignore"

// Progress lines use \r, which only makes sense on a terminal.
var stdoutIsTTY = func() bool {
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}()

// Transfers can take a long time on slow uplinks, so no overall timeout — only
// bound the wait for the server's response once the body is sent.
var transferClient = func() *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.ResponseHeaderTimeout = 5 * time.Minute
	return &http.Client{Transport: t}
}()

func isBandDir(p string) bool {
	if !strings.EqualFold(filepath.Ext(p), ".band") {
		return false
	}
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// findBands returns .band packages in dir, or dir itself if it is one.
func findBands(dir string) []string {
	if abs, err := filepath.Abs(dir); err == nil && isBandDir(abs) {
		return []string{abs}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var found []string
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		if isBandDir(p) {
			found = append(found, p)
		}
	}
	return found
}

// garageBandDir is where GarageBand saves by default; used when the binary is
// double-clicked on a Mac (cwd = home, no project around).
func garageBandDir() string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(home, "Music", "GarageBand")
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		return dir
	}
	return ""
}

// ---------------------------------------------------------------- excludes

type excluder struct {
	patterns []string
}

// loadExcluder combines the defaults, <dir of .band>/.jerboaignore and --exclude flags.
//
// Pattern rules (gitignore-lite, case-insensitive, * ? [] globs):
//   - no slash: matches any single file or folder name, e.g. `*#03.aif`
//   - with slash: matches a path from the package root, e.g. `Media/Audio Files/Take*`
//   - a matching folder excludes everything inside it
func loadExcluder(bandPath string, extra []string) *excluder {
	ex := &excluder{}
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" || strings.HasPrefix(p, "#") {
			return
		}
		p = strings.Trim(filepath.ToSlash(p), "/")
		ex.patterns = append(ex.patterns, strings.ToLower(p))
	}
	for _, p := range defaultExcludes {
		add(p)
	}
	if f, err := os.Open(filepath.Join(filepath.Dir(bandPath), ignoreFileName)); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			add(sc.Text())
		}
		f.Close()
	}
	for _, p := range extra {
		add(p)
	}
	return ex
}

func (e *excluder) match(rel string) bool {
	comps := strings.Split(strings.ToLower(rel), "/")
	for _, p := range e.patterns {
		if !strings.Contains(p, "/") {
			for _, c := range comps {
				if ok, _ := path.Match(p, c); ok {
					return true
				}
			}
			continue
		}
		for i := 1; i <= len(comps); i++ {
			if ok, _ := path.Match(p, strings.Join(comps[:i], "/")); ok {
				return true
			}
		}
	}
	return false
}

// scanBand walks the package and splits it into files to sync and excluded
// top-most paths (with their total size, so the user sees what they're saving).
func scanBand(root string, ex *excluder) ([]localFile, map[string]int64, error) {
	var files []localFile
	excluded := map[string]int64{}

	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == root {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)

		if ex.match(rel) {
			if d.IsDir() {
				excluded[rel] = dirSize(p)
				return filepath.SkipDir
			}
			if info, err := d.Info(); err == nil {
				excluded[rel] = info.Size()
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil // directories recurse; symlinks and specials are skipped
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		files = append(files, localFile{Rel: rel, Abs: p, Size: info.Size()})
		return nil
	})
	return files, excluded, err
}

func dirSize(dir string) int64 {
	var total int64
	filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

// trackGroup collapses a file into the thing a human actually decides about.
// GarageBand names recordings "<track name>#<take>.aif", so every take of one
// track shares a prefix — and the returned key doubles as a .jerboaignore
// pattern. Files elsewhere in the package group by folder.
func trackGroup(rel string) string {
	const audioDir = "Media/Audio Files/"
	if !strings.HasPrefix(rel, audioDir) {
		if dir := path.Dir(rel); dir != "." {
			return dir + "/"
		}
		return rel
	}
	base := path.Base(rel)
	if i := strings.LastIndex(base, "#"); i > 0 {
		return base[:i] + "#*"
	}
	return base
}

// excludeGroup collapses skipped takes the same way, but leaves folders like
// "Alternatives/000/Undo Data.nosync" named as themselves.
func excludeGroup(rel string) string {
	if strings.HasPrefix(rel, "Media/Audio Files/") {
		return "Media/Audio Files/" + trackGroup(rel)
	}
	return rel
}

type groupStat struct {
	key      string
	files    int
	bytes    int64
	toSend   int
	sendSize int64
}

// printGroups summarises the project by track rather than by file. With
// hundreds of takes, a per-file list is unreadable; this is the view you
// actually pick exclusions from.
func printGroups(files []localFile, sending map[string]bool) {
	byKey := map[string]*groupStat{}
	for _, f := range files {
		k := trackGroup(f.Rel)
		g := byKey[k]
		if g == nil {
			g = &groupStat{key: k}
			byKey[k] = g
		}
		g.files++
		g.bytes += f.Size
		if sending[f.Rel] {
			g.toSend++
			g.sendSize += f.Size
		}
	}

	groups := make([]*groupStat, 0, len(byKey))
	for _, g := range byKey {
		groups = append(groups, g)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].bytes > groups[j].bytes })

	fmt.Printf("\nby track (add any of these to %s to skip it):\n", ignoreFileName)
	for _, g := range groups {
		pending := ""
		if g.toSend > 0 {
			pending = fmt.Sprintf("  (%d to upload, %s)", g.toSend, humanSize(g.sendSize))
		}
		fmt.Printf("  %-40s %4d files %9s%s\n", g.key, g.files, humanSize(g.bytes), pending)
	}
}

// ---------------------------------------------------------------- push

func runBandPush(cfg *Config, bandPath string, opts mirrorOpts) {
	root, err := filepath.Abs(bandPath)
	if err != nil {
		fatalf("%v", err)
	}
	name := filepath.Base(root)
	fmt.Printf("project : %s (GarageBand)\n", name)
	if opts.DryRun {
		fmt.Printf("dry run : nothing will be uploaded or deleted\n")
	}
	warnIfGarageBandRunning()
	keepAwake()

	cache := loadHashCache(root)
	defer cache.save()

	ex := loadExcluder(root, opts.Excludes)
	files, excluded, err := scanBand(root, ex)
	if err != nil {
		fatalf("scan %s: %v", name, err)
	}

	remote, err := listMirrorFiles(cfg, name)
	if err != nil {
		fatalf("get state: %v", err)
	}
	remoteByPath := map[string]mirrorFile{}
	for _, f := range remote {
		remoteByPath[f.Path] = f
	}

	if len(excluded) > 0 {
		fmt.Printf("\nexcluded (edit %s next to the .band to change):\n", ignoreFileName)
		grouped := map[string]int64{}
		counts := map[string]int{}
		for p, size := range excluded {
			k := excludeGroup(p)
			grouped[k] += size
			counts[k]++
		}
		paths := make([]string, 0, len(grouped))
		for p := range grouped {
			paths = append(paths, p)
		}
		sort.Slice(paths, func(i, j int) bool { return grouped[paths[i]] > grouped[paths[j]] })
		var total int64
		for _, p := range paths {
			label := p
			if counts[p] > 1 {
				label = fmt.Sprintf("%s (%d files)", p, counts[p])
			}
			fmt.Printf("  skip  %-50s %9s\n", label, humanSize(grouped[p]))
			total += grouped[p]
		}
		fmt.Printf("        %-50s %9s\n", "total", humanSize(total))
	}

	fmt.Printf("\n%d file(s) to check\n", len(files))
	if opts.Verbose || len(files) <= 40 {
		fmt.Println()
	}

	type pending struct {
		f    localFile
		hash string
	}
	var toSend []pending
	var sendBytes int64
	local := map[string]bool{}
	failed := 0

	for _, f := range files {
		local[f.Rel] = true
		hash, err := cache.hash(f)
		if err != nil {
			fmt.Printf("  error %s: %v\n", f.Rel, err)
			failed++
			continue
		}
		// With hundreds of takes a per-file list is noise; --list forces it.
		verbose := opts.Verbose || len(files) <= 40
		r, onServer := remoteByPath[f.Rel]
		switch {
		case onServer && r.Hash == hash:
			if verbose {
				fmt.Printf("  ok    %s\n", f.Rel)
			}
		case onServer:
			if verbose {
				fmt.Printf("  chg   %-50s %9s\n", f.Rel, humanSize(f.Size))
			}
			toSend = append(toSend, pending{f, hash})
			sendBytes += f.Size
		default:
			if verbose {
				fmt.Printf("  new   %-50s %9s\n", f.Rel, humanSize(f.Size))
			}
			toSend = append(toSend, pending{f, hash})
			sendBytes += f.Size
		}
	}

	var toDelete []string
	for _, r := range remote {
		if !local[r.Path] {
			fmt.Printf("  del   %s\n", r.Path)
			toDelete = append(toDelete, r.Path)
		}
	}

	sending := map[string]bool{}
	for _, p := range toSend {
		sending[p.f.Rel] = true
	}
	printGroups(files, sending)

	fmt.Printf("\n%d to upload (%s), %d to remove from server, %d unchanged\n",
		len(toSend), humanSize(sendBytes), len(toDelete), len(files)-len(toSend)-failed)
	if opts.DryRun {
		waitIfWindows()
		return
	}
	if len(toSend) > 0 {
		fmt.Println()
	}

	started := time.Now()
	var sentBytes int64
	uploaded := 0
	for _, p := range toSend {
		// On a retry, re-hash first: GarageBand may have re-saved the file
		// under us, and uploading bytes that don't match the committed hash
		// would leave a backup we can't verify later.
		err := withRetry("up    "+p.f.Rel, func(attempt int) error {
			if attempt > 0 {
				cache.forget(p.f.Rel)
				h, err := cache.hash(p.f)
				if err != nil {
					return err
				}
				if info, err := os.Stat(p.f.Abs); err == nil {
					p.f.Size = info.Size()
				}
				p.hash = h
			}
			return putMirrorFile(cfg, name, p.f, p.hash)
		})
		if err != nil {
			fmt.Printf("\r  up    %s  FAILED: %v\n", p.f.Rel, err)
			failed++
			if isFatal(err) {
				fmt.Printf("\nstopping — this affects every file, so there's no point continuing.\n")
				if hint := fatalHint(err); hint != "" {
					fmt.Printf("%s\n", hint)
				}
				break
			}
			continue
		}
		fmt.Printf("\r  up    %-50s %9s  done                    \n", p.f.Rel, humanSize(p.f.Size))
		sentBytes += p.f.Size
		uploaded++
	}

	deleted := 0
	for _, rel := range toDelete {
		if err := withRetry("del   "+rel, func(int) error { return deleteMirrorFile(cfg, name, rel) }); err != nil {
			fmt.Printf("  del   %s  FAILED: %v\n", rel, err)
			failed++
			continue
		}
		deleted++
	}

	fmt.Printf("\npush — %d uploaded, %d removed from server, %d failed\n", uploaded, deleted, failed)
	if sentBytes > 0 {
		elapsed := time.Since(started)
		fmt.Printf("       %s in %s (%s/s average)\n", humanSize(sentBytes), elapsed.Round(time.Second),
			humanSize(int64(float64(sentBytes)/elapsed.Seconds())))
	}
	waitIfWindows()
	if failed > 0 {
		os.Exit(1)
	}
}

// ---------------------------------------------------------------- pull

// runBandPull downloads a mirrored project into destDir/<name>. Changed local
// files are moved to a timestamped backup folder first; files that exist only
// locally are left alone.
func runBandPull(cfg *Config, name, destDir string, opts mirrorOpts) {
	root := filepath.Join(destDir, name)
	fmt.Printf("pulling %s → %s\n", name, root)
	if opts.DryRun {
		fmt.Printf("dry run : nothing will be downloaded\n")
	}
	warnIfGarageBandRunning()
	keepAwake()

	remote, err := listMirrorFiles(cfg, name)
	if err != nil {
		fatalf("get state: %v", err)
	}
	ex := loadExcluder(root, opts.Excludes)
	backupDir := root + ".jerboa-backup-" + time.Now().Format("20060102-150405")
	backedUp := false

	fmt.Println()
	downloaded, skipped, failed := 0, 0, 0
	onServer := map[string]bool{}

	for _, f := range remote {
		onServer[f.Path] = true
		rel := filepath.FromSlash(f.Path)
		if !filepath.IsLocal(rel) {
			fmt.Printf("  skip  %s (unsafe path)\n", f.Path)
			continue
		}
		if ex.match(f.Path) {
			skipped++
			continue
		}
		dest := filepath.Join(root, rel)

		exists := false
		if _, err := os.Stat(dest); err == nil {
			exists = true
			if h, err := hashFile(dest); err == nil && h == f.Hash {
				fmt.Printf("  ok    %s\n", f.Path)
				skipped++
				continue
			}
		}

		label := "dl  "
		if exists {
			label = "chg "
		}
		if opts.DryRun {
			fmt.Printf("  %s  %-50s %9s\n", label, f.Path, humanSize(f.Size))
			continue
		}

		if exists {
			bak := filepath.Join(backupDir, rel)
			if err := os.MkdirAll(filepath.Dir(bak), 0755); err == nil {
				err = os.Rename(dest, bak)
			}
			if err != nil {
				fmt.Printf("  %s  %s  FAILED: backup: %v\n", label, f.Path, err)
				failed++
				continue
			}
			backedUp = true
		}

		if err := withRetry(label+"  "+f.Path, func(int) error { return getMirrorFile(cfg, name, f, dest, label) }); err != nil {
			fmt.Printf("\r  %s  %s  FAILED: %v\n", label, f.Path, err)
			failed++
			continue
		}
		fmt.Printf("\r  %s  %-50s %9s  done\n", label, f.Path, humanSize(f.Size))
		downloaded++
	}

	if files, _, err := scanBand(root, ex); err == nil {
		localOnly := 0
		for _, f := range files {
			if !onServer[f.Rel] {
				localOnly++
			}
		}
		if localOnly > 0 {
			fmt.Printf("\n%d local file(s) not on the server were left alone\n", localOnly)
		}
	}

	fmt.Printf("\npull — %d downloaded, %d skipped, %d failed\n", downloaded, skipped, failed)
	if backedUp {
		fmt.Printf("your previous versions of changed files are in %s\n", backupDir)
	}
	waitIfWindows()
	if failed > 0 {
		os.Exit(1)
	}
}

// ---------------------------------------------------------------- http

func mirrorURL(cfg *Config, endpoint string, q url.Values) string {
	return fmt.Sprintf("%s/api/bands/%s/mirror/%s?%s", cfg.ServerURL, cfg.BandSlug, endpoint, q.Encode())
}

func listMirrorProjects(cfg *Config) ([]mirrorProject, error) {
	var result struct {
		Projects []mirrorProject `json:"projects"`
	}
	return result.Projects, getJSON(mirrorURL(cfg, "projects", nil), cfg, &result)
}

func listMirrorFiles(cfg *Config, project string) ([]mirrorFile, error) {
	var result struct {
		Files []mirrorFile `json:"files"`
	}
	return result.Files, getJSON(mirrorURL(cfg, "files", url.Values{"project": {project}}), cfg, &result)
}

type mirrorFileRequest struct {
	Project string `json:"project"`
	Path    string `json:"path"`
	Hash    string `json:"hash"`
	Size    int64  `json:"size"`
}

func postJSON(u string, cfg *Config, payload, v any) error {
	buf, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, _ := http.NewRequest("POST", u, bytes.NewReader(buf))
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := transferClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return &httpError{Status: resp.StatusCode, Body: strings.TrimSpace(string(msg))}
	}
	if v == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

func getJSON(u string, cfg *Config, v any) error {
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	resp, err := transferClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return &httpError{Status: resp.StatusCode, Body: strings.TrimSpace(string(body))}
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// putMirrorFile uploads straight to the backup bucket with a presigned URL,
// then tells Jerboa about it. The bytes never touch the Jerboa server.
func putMirrorFile(cfg *Config, project string, f localFile, hash string) error {
	req := mirrorFileRequest{Project: project, Path: f.Rel, Hash: hash, Size: f.Size}

	var presigned struct {
		URL string `json:"url"`
	}
	if err := postJSON(mirrorURL(cfg, "upload-url", nil), cfg, req, &presigned); err != nil {
		return err
	}

	file, err := os.Open(f.Abs)
	if err != nil {
		return err
	}
	defer file.Close()

	// Presigned URLs carry their own signature — no Authorization header, and
	// no extra headers that weren't signed.
	ctx, watchdog := newStallWatchdog(context.Background())
	defer watchdog.stop()

	body := &progressReader{r: file, total: f.Size, label: "up    " + f.Rel, watchdog: watchdog}
	put, _ := http.NewRequestWithContext(ctx, "PUT", presigned.URL, body)
	put.ContentLength = f.Size

	resp, err := transferClient.Do(put)
	if err != nil {
		return transferError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return &httpError{Status: resp.StatusCode, Body: strings.TrimSpace(string(msg)), Source: "bucket"}
	}

	// Commit — the server checks the object really landed at the right size.
	return postJSON(mirrorURL(cfg, "commit", nil), cfg, req, nil)
}

func deleteMirrorFile(cfg *Config, project, rel string) error {
	req, _ := http.NewRequest("DELETE", mirrorURL(cfg, "files", url.Values{"project": {project}, "path": {rel}}), nil)
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	resp, err := transferClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return &httpError{Status: resp.StatusCode, Body: strings.TrimSpace(string(msg))}
	}
	return nil
}

// getMirrorFile downloads to a temp file beside dest, checks the hash, then
// renames into place so an interrupted pull never leaves a half-written file.
func getMirrorFile(cfg *Config, project string, f mirrorFile, dest, label string) error {
	var presigned struct {
		URL string `json:"url"`
	}
	q := url.Values{"project": {project}, "path": {f.Path}}
	if err := getJSON(mirrorURL(cfg, "download-url", q), cfg, &presigned); err != nil {
		return err
	}

	ctx, watchdog := newStallWatchdog(context.Background())
	defer watchdog.stop()

	req, _ := http.NewRequestWithContext(ctx, "GET", presigned.URL, nil)
	resp, err := transferClient.Do(req)
	if err != nil {
		return transferError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return &httpError{Status: resp.StatusCode, Body: strings.TrimSpace(string(msg)), Source: "bucket"}
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	tmp := dest + ".jerboa-part"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, &progressReader{r: resp.Body, total: f.Size, label: label + "  " + f.Path, watchdog: watchdog})
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		var h string
		if h, err = hashFile(tmp); err == nil && h != f.Hash {
			err = fmt.Errorf("hash mismatch")
		}
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dest)
}

// ---------------------------------------------------------------- helpers

type progressReader struct {
	r         io.Reader
	total     int64
	done      int64
	label     string
	started   time.Time
	lastPrint time.Time
	watchdog  *stallWatchdog
}

// Read reports percent, throughput and ETA — the numbers you need to tell a
// slow link from a slow machine.
func (p *progressReader) Read(b []byte) (int, error) {
	if p.started.IsZero() {
		p.started = time.Now()
	}
	n, err := p.r.Read(b)
	p.done += int64(n)
	if p.watchdog != nil {
		p.watchdog.progress(int64(n))
	}
	// Redirected to a log file, a live progress line is just noise — and an
	// overnight run's log is the thing we ask the user to send back.
	if stdoutIsTTY && p.total > 0 && time.Since(p.lastPrint) > 250*time.Millisecond {
		p.lastPrint = time.Now()
		elapsed := time.Since(p.started).Seconds()
		if elapsed <= 0 {
			return n, err
		}
		rate := float64(p.done) / elapsed
		eta := time.Duration(float64(p.total-p.done) / rate * float64(time.Second))
		fmt.Printf("\r  %s  %3d%%  %s/s  eta %s   ", p.label, p.done*100/p.total,
			humanSize(int64(rate)), eta.Round(time.Second))
	}
	return n, err
}

func warnIfGarageBandRunning() {
	if runtime.GOOS != "darwin" {
		return
	}
	if exec.Command("pgrep", "-x", "GarageBand").Run() == nil {
		fmt.Printf("\nwarning : GarageBand is open — save and close the project first, or files may be mid-write\n")
	}
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
