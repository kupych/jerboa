package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// DistFS returns the embedded frontend filesystem.
// Returns nil if no frontend build is present.
func DistFS() fs.FS {
	// Check if a real build exists (not just .keep)
	if _, err := dist.ReadFile("dist/index.html"); err != nil {
		return nil
	}
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil
	}
	return sub
}
