//go:build dev

package webui

import (
	"io/fs"
	"testing/fstest"
)

// Dist is a minimal placeholder used by backend-only development runs.
// The real admin UI is served by Nuxt during `make frontend-dev`.
var Dist fs.FS = fstest.MapFS{
	"dist/index.html": &fstest.MapFile{
		Data: []byte("<!doctype html><html><head><title>MeowCLI Dev</title></head><body>Run the Nuxt dev server for the admin UI.</body></html>"),
	},
}
