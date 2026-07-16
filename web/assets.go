// Package webui owns the compiled dashboard embedded in release binaries.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:build
var embedded embed.FS

// FS returns the compiled dashboard rooted at its index.html.
func FS() (fs.FS, error) {
	return fs.Sub(embedded, "build")
}
