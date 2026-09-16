package server

import (
	_ "embed"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/go-chi/chi/v5"
)

// The one-line installer and the token-free binary it downloads. Both are
// public: the binary carries no credentials, and a computer only gets a token
// by pairing, which a signed-in band member approves in the browser.

//go:embed install.sh.tmpl
var installScriptSrc string

var installScript = template.Must(template.New("install.sh").Parse(installScriptSrc))

// The server URL is spliced into a shell script, so only accept characters a
// URL needs. It comes from the Host header, which a client controls.
var safeBaseURL = regexp.MustCompile(`^https?://[A-Za-z0-9.\-]+(:[0-9]+)?$`)

// InstallScript serves the installer with this server's URL filled in.
// GET /install.sh
func (h *SyncHandler) InstallScript(w http.ResponseWriter, r *http.Request) {
	base := publicBaseURL(r)
	if !safeBaseURL.MatchString(base) {
		http.Error(w, "unexpected host", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := installScript.Execute(w, struct{ ServerURL string }{base}); err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
	}
}

// GenericBinary serves a sync binary with no embedded config. It pairs on
// first run instead.
// GET /dl/jerboa-sync/{platform}
func (h *SyncHandler) GenericBinary(w http.ResponseWriter, r *http.Request) {
	platform := chi.URLParam(r, "platform")
	name, ok := syncBinNames[platform]
	if !ok {
		http.Error(w, "unknown platform", http.StatusNotFound)
		return
	}
	f, err := os.Open(filepath.Join(h.binDir, name))
	if err != nil {
		http.Error(w, "sync binary not built — run make build-sync on the server", http.StatusNotFound)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}

	download := "jerboa-sync"
	if strings.HasSuffix(name, ".exe") {
		download += ".exe"
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, download))
	// ServeContent handles Range requests, so an interrupted download resumes.
	http.ServeContent(w, r, download, info.ModTime(), f)
}
