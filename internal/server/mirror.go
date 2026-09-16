package server

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"

	"jerboa/internal/db"
)

// Project mirror: file-by-file sync of DAW project packages whose session
// format we can't parse (GarageBand .band). The bytes live in a separate
// bucket (Backblaze B2) with versioning on, so the mirror doubles as the
// band's off-site backup: overwriting a file keeps the old version, and a
// bucket lifecycle rule ages prior versions out after 30 days.
//
// Clients upload straight to the bucket with a presigned URL and then commit
// the result here, so project files never transit the server.

// Long enough that a multi-gigabyte file on a slow domestic uplink can't have
// its URL expire mid-transfer. An expired signature and bad credentials both
// come back as 403, and the client treats bucket 403s as permanent, so this
// window has to comfortably outlast the slowest realistic upload.
const mirrorURLTTL = 12 * time.Hour

// mirrorObjectKey is deterministic: overwriting the same key is what gives us
// version history in the bucket.
func mirrorObjectKey(bandID uuid.UUID, project, relPath string) string {
	return path.Join("mirror", bandID.String(), project, relPath)
}

type mirrorFileRequest struct {
	Project string `json:"project"`
	Path    string `json:"path"`
	Hash    string `json:"hash"`
	Size    int64  `json:"size"`
}

// MirrorProjects lists mirrored projects for a band.
// GET /api/bands/{slug}/mirror/projects
func (h *SyncHandler) MirrorProjects(w http.ResponseWriter, r *http.Request) {
	band, ok := h.getBand(w, r, UserFrom(r.Context()))
	if !ok {
		return
	}
	projects, err := h.queries.ListMirrorProjects(r.Context(), band.ID)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if projects == nil {
		projects = []db.MirrorProject{}
	}
	writeJSON(w, map[string]any{"projects": projects})
}

// MirrorFiles lists path/hash/size for every file in a project.
// GET /api/bands/{slug}/mirror/files?project=
func (h *SyncHandler) MirrorFiles(w http.ResponseWriter, r *http.Request) {
	band, ok := h.getBand(w, r, UserFrom(r.Context()))
	if !ok {
		return
	}
	project, ok := mirrorProjectParam(w, r.URL.Query().Get("project"))
	if !ok {
		return
	}
	files, err := h.queries.ListMirrorFiles(r.Context(), band.ID, project)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if files == nil {
		files = []db.MirrorFile{}
	}
	writeJSON(w, map[string]any{"files": files})
}

// MirrorUploadURL hands out a presigned PUT for one file.
// POST /api/bands/{slug}/mirror/upload-url  {project, path, hash, size}
func (h *SyncHandler) MirrorUploadURL(w http.ResponseWriter, r *http.Request) {
	band, ok := h.getBand(w, r, UserFrom(r.Context()))
	if !ok {
		return
	}
	if !h.mirrorConfigured(w) {
		return
	}
	req, ok := h.decodeMirrorRequest(w, r, true)
	if !ok {
		return
	}

	key := mirrorObjectKey(band.ID, req.Project, req.Path)
	url, err := h.mirrorS3.PresignedPutURL(r.Context(), key, mirrorURLTTL)
	if err != nil {
		slog.Error("mirror: presign put", "key", key, "error", err)
		http.Error(w, `{"error":"could not presign upload"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"url": url, "key": key})
}

// MirrorCommit records a file after the client uploaded it to the bucket. The
// server checks the object is actually there and the right size, so a failed
// or truncated upload can't leave a row claiming a backup exists.
// POST /api/bands/{slug}/mirror/commit  {project, path, hash, size}
func (h *SyncHandler) MirrorCommit(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	band, ok := h.getBand(w, r, user)
	if !ok {
		return
	}
	if !h.mirrorConfigured(w) {
		return
	}
	req, ok := h.decodeMirrorRequest(w, r, true)
	if !ok {
		return
	}

	key := mirrorObjectKey(band.ID, req.Project, req.Path)
	size, err := h.mirrorS3.Stat(r.Context(), key)
	if err != nil {
		http.Error(w, `{"error":"object not found in storage"}`, http.StatusBadRequest)
		return
	}
	if size != req.Size {
		http.Error(w, fmt.Sprintf(`{"error":"size mismatch: uploaded %d, expected %d"}`, size, req.Size), http.StatusBadRequest)
		return
	}

	if err := h.queries.UpsertMirrorFile(r.Context(), band.ID, user.ID, req.Project, db.MirrorFile{
		Path: req.Path, Hash: req.Hash, Size: req.Size, ObjectKey: key,
	}); err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// MirrorDownloadURL hands out a presigned GET for one file.
// GET /api/bands/{slug}/mirror/download-url?project=&path=
func (h *SyncHandler) MirrorDownloadURL(w http.ResponseWriter, r *http.Request) {
	band, ok := h.getBand(w, r, UserFrom(r.Context()))
	if !ok {
		return
	}
	if !h.mirrorConfigured(w) {
		return
	}
	project, ok := mirrorProjectParam(w, r.URL.Query().Get("project"))
	if !ok {
		return
	}
	relPath, ok := mirrorPathParam(w, r.URL.Query().Get("path"))
	if !ok {
		return
	}

	f, err := h.queries.GetMirrorFile(r.Context(), band.ID, project, relPath)
	if err != nil || f == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	url, err := h.mirrorS3.PresignedURL(r.Context(), f.ObjectKey, mirrorURLTTL)
	if err != nil {
		slog.Error("mirror: presign get", "key", f.ObjectKey, "error", err)
		http.Error(w, `{"error":"could not presign download"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"url": url, "hash": f.Hash, "size": f.Size})
}

// MirrorDelete drops a file the client no longer has (or newly excluded).
// The object's prior versions stay in the bucket until the lifecycle rule
// expires them, so an over-eager exclude is recoverable.
// DELETE /api/bands/{slug}/mirror/files?project=&path=
func (h *SyncHandler) MirrorDelete(w http.ResponseWriter, r *http.Request) {
	band, ok := h.getBand(w, r, UserFrom(r.Context()))
	if !ok {
		return
	}
	if !h.mirrorConfigured(w) {
		return
	}
	project, ok := mirrorProjectParam(w, r.URL.Query().Get("project"))
	if !ok {
		return
	}
	relPath, ok := mirrorPathParam(w, r.URL.Query().Get("path"))
	if !ok {
		return
	}

	key, err := h.queries.DeleteMirrorFile(r.Context(), band.ID, project, relPath)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if key != "" {
		if err := h.mirrorS3.Delete(r.Context(), key); err != nil {
			slog.Warn("mirror: delete object", "key", key, "error", err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// mirrorConfigured reports 501 rather than 503 on purpose: the feature is off
// until someone sets JERBOA_MIRROR_S3_*, so it is permanent from the client's
// point of view and must not be retried. Matches files.go's handling of
// unconfigured object storage.
func (h *SyncHandler) mirrorConfigured(w http.ResponseWriter) bool {
	if h.mirrorS3 == nil {
		http.Error(w, `{"error":"project mirror storage is not configured on this server (JERBOA_MIRROR_S3_* unset)"}`, http.StatusNotImplemented)
		return false
	}
	return true
}

func (h *SyncHandler) decodeMirrorRequest(w http.ResponseWriter, r *http.Request, needHash bool) (*mirrorFileRequest, bool) {
	var req mirrorFileRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return nil, false
	}
	if _, ok := mirrorProjectParam(w, req.Project); !ok {
		return nil, false
	}
	if _, ok := mirrorPathParam(w, req.Path); !ok {
		return nil, false
	}
	if needHash && len(req.Hash) != sha256.Size*2 {
		http.Error(w, `{"error":"hash required"}`, http.StatusBadRequest)
		return nil, false
	}
	if req.Size < 0 || req.Size > h.mirrorMaxBytes {
		http.Error(w, fmt.Sprintf(`{"error":"file too large (limit %d MB)"}`, h.mirrorMaxBytes>>20), http.StatusRequestEntityTooLarge)
		return nil, false
	}
	req.Hash = strings.ToLower(req.Hash)
	return &req, true
}

func mirrorProjectParam(w http.ResponseWriter, name string) (string, bool) {
	if name == "" || len(name) > 255 || strings.ContainsAny(name, "/\\\x00") || name == "." || name == ".." {
		http.Error(w, `{"error":"invalid project"}`, http.StatusBadRequest)
		return "", false
	}
	return name, true
}

// mirrorPathParam accepts only clean, relative, slash-separated paths so a
// pulled project can never write outside its own folder, and so no client can
// reach another band's objects through the key prefix.
func mirrorPathParam(w http.ResponseWriter, p string) (string, bool) {
	if p == "" || len(p) > 1024 || strings.ContainsAny(p, "\\\x00") || path.IsAbs(p) ||
		path.Clean(p) != p || p == ".." || strings.HasPrefix(p, "../") {
		http.Error(w, `{"error":"invalid path"}`, http.StatusBadRequest)
		return "", false
	}
	return p, true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
