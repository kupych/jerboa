package server

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"jerboa/internal/storage"
)

const maxFieldBytes = 4 << 10

type streamedUpload struct {
	FilePath string
	FileSize int64
	Filename string
	Fields   map[string]string
}

// streamMultipartToStorage reads a multipart request and streams the first file
// part directly into storage, avoiding ParseMultipartForm's spill-to-tempfile
// behaviour for large bodies. Non-file fields are collected into a map (capped
// per field at maxFieldBytes).
//
// Clients must send the "file" part first (uploadFile() in api.ts does this);
// any later parts are read as fields after Save returns.
func streamMultipartToStorage(w http.ResponseWriter, r *http.Request, store *storage.Store, bandID uuid.UUID, maxBytes int64) (*streamedUpload, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, fmt.Errorf("not a multipart request: %w", err)
	}

	res := &streamedUpload{Fields: make(map[string]string)}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return res, err
		}
		if err := readPart(part, store, bandID, res); err != nil {
			return res, err
		}
	}

	if res.FilePath == "" {
		return res, errors.New("file is required")
	}
	return res, nil
}

func readPart(part *multipart.Part, store *storage.Store, bandID uuid.UUID, res *streamedUpload) error {
	defer part.Close()

	if filename := part.FileName(); filename != "" {
		if res.FilePath != "" {
			_, _ = io.Copy(io.Discard, part)
			return nil
		}
		path, size, err := store.Save(bandID, filename, part)
		if err != nil {
			return err
		}
		res.FilePath = path
		res.FileSize = size
		res.Filename = filename
		return nil
	}

	name := part.FormName()
	var buf strings.Builder
	if _, err := io.Copy(&buf, io.LimitReader(part, maxFieldBytes)); err != nil {
		return err
	}
	res.Fields[name] = buf.String()
	return nil
}
