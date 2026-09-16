package main

// Credentials and memory for installs that came from install.sh, where the
// binary has no token baked in: it pairs with a band once, then remembers the
// projects you've synced so a double-click can re-sync them.

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type storedConfig struct {
	ServerURL string `json:"server_url,omitempty"`
	BandSlug  string `json:"band_slug,omitempty"`
	Token     string `json:"token,omitempty"`

	// InstallID distinguishes this computer's copies of a project from
	// same-named projects elsewhere. See projectSource.
	InstallID string `json:"install_id,omitempty"`

	// Projects are paths the user has chosen to sync: .band packages, or
	// folders whose .band packages should all be synced.
	Projects []string `json:"projects,omitempty"`
}

// stored is loaded once at startup and saved whenever it changes.
var stored = &storedConfig{}

func configPath() (string, error) {
	dir, err := os.UserConfigDir() // ~/Library/Application Support on a Mac
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "jerboa-sync", "config.json"), nil
}

func loadStored() *storedConfig {
	c := &storedConfig{}
	if p, err := configPath(); err == nil {
		if data, err := os.ReadFile(p); err == nil {
			json.Unmarshal(data, c)
		}
	}
	if c.InstallID == "" {
		b := make([]byte, 16)
		rand.Read(b)
		c.InstallID = hex.EncodeToString(b)
		if err := saveStored(c); err != nil {
			fmt.Fprintf(os.Stderr, "warning: couldn't save settings: %v\n", err)
		}
	}
	return c
}

// saveStored writes atomically with owner-only permissions: the file holds
// an API token.
func saveStored(c *storedConfig) error {
	p, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// resolveConfig prefers credentials baked into a binary downloaded from the
// band settings page, so those copies keep working, then falls back to pairing.
func resolveConfig() (*Config, error) {
	if cfg, err := loadEmbeddedConfig(); err == nil {
		return cfg, nil
	}
	if stored.Token == "" || stored.ServerURL == "" || stored.BandSlug == "" {
		return nil, errors.New("this computer isn't connected to a band yet")
	}
	return &Config{ServerURL: stored.ServerURL, BandSlug: stored.BandSlug, Token: stored.Token}, nil
}

// ---------------------------------------------------------------- projects

type projectSourceInfo struct {
	ID    string
	Label string
}

// projectSource identifies this particular copy of a project: this install
// plus its absolute path. The server only knows projects by name, so this is
// what tells "the same project again" apart from "a different My Song.band".
func projectSource(root string) projectSourceInfo {
	sum := sha256.Sum256([]byte(stored.InstallID + "\x00" + root))
	label := root
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(root, home+string(filepath.Separator)) {
		label = "~" + strings.TrimPrefix(root, home)
	}
	if host, err := os.Hostname(); err == nil {
		label = host + ": " + label
	}
	return projectSourceInfo{ID: hex.EncodeToString(sum[:16]), Label: label}
}

func rememberProject(path string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return
	}
	for _, p := range stored.Projects {
		if p == abs {
			return
		}
	}
	stored.Projects = append(stored.Projects, abs)
	if err := saveStored(stored); err != nil {
		fmt.Printf("warning: couldn't remember %s: %v\n", filepath.Base(abs), err)
	}
}

func forgetProject(path string) bool {
	abs, _ := filepath.Abs(path)
	kept := stored.Projects[:0]
	found := false
	for _, p := range stored.Projects {
		if p == abs || p == path {
			found = true
			continue
		}
		kept = append(kept, p)
	}
	stored.Projects = kept
	if found {
		saveStored(stored)
	}
	return found
}

// expandTarget turns a chosen path into the .band packages it means: itself if
// it is one, or every .band directly inside it if it's a folder.
func expandTarget(p string) ([]string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, errors.New("not found (moved or deleted?)")
	}
	if !info.IsDir() {
		return nil, errors.New("not a GarageBand project or a folder")
	}
	bands := findBands(abs)
	if len(bands) == 0 {
		return nil, errors.New("no GarageBand projects in this folder")
	}
	return bands, nil
}

// discoveryDirs are where GarageBand projects tend to live if nothing has been
// remembered yet: the default save folder and iCloud Drive.
func discoveryDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	var dirs []string
	if dir := garageBandDir(); dir != "" {
		dirs = append(dirs, dir)
	}
	if runtime.GOOS == "darwin" {
		icloud := filepath.Join(home, "Library", "Mobile Documents", "com~apple~CloudDocs")
		if entries, err := os.ReadDir(icloud); err == nil {
			dirs = append(dirs, icloud)
			for _, e := range entries {
				if e.IsDir() && strings.Contains(strings.ToLower(e.Name()), "garageband") {
					dirs = append(dirs, filepath.Join(icloud, e.Name()))
				}
			}
		}
	}
	return dirs
}

// ---------------------------------------------------------------- prompts

var stdin = bufio.NewReader(os.Stdin)

// readLine returns "" at end of input, so an unattended run — cron, launchd —
// takes each prompt's default.
func readLine(prompt string) string {
	fmt.Print(prompt)
	line, _ := stdin.ReadString('\n')
	return strings.TrimSpace(line)
}

// confirm requires the word "yes": it guards replacing someone else's backup,
// and at end of input it answers no.
func confirm(prompt string) bool {
	return strings.EqualFold(readLine(prompt), "yes")
}

// ---------------------------------------------------------------- pairing

// runPair links this computer to a band the same way the Reaper script does:
// get a code, have a signed-in member approve it in the browser, and collect
// a token once they have.
func runPair(server string) error {
	server = strings.TrimRight(strings.TrimSpace(server), "/")
	if server == "" {
		return errors.New("need the server address, e.g. jerboa-sync pair --server https://jerboa.dad")
	}

	var start struct {
		Code      string `json:"code"`
		ExpiresIn int    `json:"expires_in_seconds"`
	}
	if status, err := pairRequest(server+"/api/pair/start", nil, &start); err != nil {
		return fmt.Errorf("couldn't reach %s: %w", server, err)
	} else if status != http.StatusOK {
		return fmt.Errorf("%s refused to start pairing (%d)", server, status)
	}

	// Built from the server we're talking to rather than trusting the
	// response's verify_url, which reflects the server's own configured URL.
	link := server + "/connect?code=" + start.Code
	fmt.Printf("\nConnect this computer to your band:\n\n")
	fmt.Printf("  1. Your browser should open. If it doesn't, go to:\n       %s\n", link)
	fmt.Printf("  2. Sign in if asked, pick your band, and click authorize.\n")
	fmt.Printf("     The code should read %s.\n\n", start.Code)
	openBrowser(link)
	fmt.Print("Waiting")

	expires := time.Duration(start.ExpiresIn) * time.Second
	if expires <= 0 {
		expires = 10 * time.Minute
	}
	deadline := time.Now().Add(expires)
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		fmt.Print(".")

		var poll struct {
			Status   string `json:"status"`
			Token    string `json:"token"`
			BandSlug string `json:"band_slug"`
		}
		status, err := pairRequest(server+"/api/pair/poll", map[string]string{"code": start.Code}, &poll)
		switch {
		case err != nil:
			continue // wifi blip while they're in the browser; keep waiting
		case status == http.StatusTooManyRequests:
			time.Sleep(5 * time.Second)
			continue
		case status == http.StatusNotFound:
			fmt.Println()
			return errors.New("that code expired or was already used — run the installer again")
		case status != http.StatusOK:
			continue
		case poll.Status == "authorized":
			stored.ServerURL = server
			stored.BandSlug = poll.BandSlug
			stored.Token = poll.Token
			if err := saveStored(stored); err != nil {
				fmt.Println()
				return fmt.Errorf("connected, but couldn't save the connection: %w", err)
			}
			fmt.Printf("\n\nConnected to your band %q.\n", poll.BandSlug)
			return nil
		}
	}
	fmt.Println()
	return errors.New("timed out waiting for approval — run the installer again")
}

// pairRequest POSTs JSON without credentials (pairing is how we get them).
func pairRequest(url string, body, v any) (int, error) {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return 0, err
		}
	}
	req, _ := http.NewRequest("POST", url, &buf)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK && v != nil {
		if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
			return resp.StatusCode, err
		}
	} else {
		io.Copy(io.Discard, resp.Body)
	}
	return resp.StatusCode, nil
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err == nil {
		go cmd.Wait()
	}
}

// runStatus exits 0 when connected and the server accepts our token, so the
// installer can skip pairing on an update. 1: not paired. 2: token rejected.
// 3: server unreachable.
func runStatus() int {
	cfg, err := resolveConfig()
	if err != nil {
		fmt.Println("not connected to a band — run the installer, or: jerboa-sync pair --server https://your-jerboa-server")
		return 1
	}
	req, _ := http.NewRequest("GET", mirrorURL(cfg, "projects", nil), nil)
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("couldn't reach %s: %v\n", cfg.ServerURL, err)
		return 3
	}
	resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		fmt.Printf("connected to %q on %s\n", cfg.BandSlug, cfg.ServerURL)
		return 0
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
		fmt.Printf("%s no longer accepts this computer's connection — run the installer again\n", cfg.ServerURL)
		return 2
	}
	fmt.Printf("unexpected response from %s: %d\n", cfg.ServerURL, resp.StatusCode)
	return 3
}
