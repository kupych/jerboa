package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

type Store struct {
	basePath string
}

func New(basePath string) (*Store, error) {
	if err := os.MkdirAll(basePath, 0750); err != nil {
		return nil, fmt.Errorf("create storage directory: %w", err)
	}
	return &Store{basePath: basePath}, nil
}

func (s *Store) Save(bandID uuid.UUID, filename string, r io.Reader) (string, int64, error) {
	dir := filepath.Join(s.basePath, bandID.String())
	if err := os.MkdirAll(dir, 0750); err != nil {
		return "", 0, err
	}

	// Use a unique ID prefix to avoid collisions
	storedName := uuid.New().String() + filepath.Ext(filename)
	fullPath := filepath.Join(dir, storedName)

	f, err := os.Create(fullPath)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	n, err := io.Copy(f, r)
	if err != nil {
		os.Remove(fullPath)
		return "", 0, err
	}

	return fullPath, n, nil
}

func (s *Store) Root() string {
	return s.basePath
}

// Import moves a file from a temporary location into permanent storage.
func (s *Store) Import(bandID uuid.UUID, filename string, srcPath string) (string, int64, error) {
	dir := filepath.Join(s.basePath, bandID.String())
	if err := os.MkdirAll(dir, 0750); err != nil {
		return "", 0, err
	}

	storedName := uuid.New().String() + filepath.Ext(filename)
	fullPath := filepath.Join(dir, storedName)

	// Try rename first (same filesystem), fall back to copy
	if err := os.Rename(srcPath, fullPath); err != nil {
		src, err := os.Open(srcPath)
		if err != nil {
			return "", 0, err
		}
		defer src.Close()

		dst, err := os.Create(fullPath)
		if err != nil {
			return "", 0, err
		}
		defer dst.Close()

		if _, err := io.Copy(dst, src); err != nil {
			os.Remove(fullPath)
			return "", 0, err
		}
		os.Remove(srcPath)
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		return "", 0, err
	}

	return fullPath, info.Size(), nil
}

func (s *Store) Open(path string) (*os.File, error) {
	return os.Open(path)
}

func (s *Store) Delete(path string) error {
	return os.Remove(path)
}
