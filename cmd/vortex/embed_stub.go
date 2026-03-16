//go:build !embed_ui

package main

import "io/fs"

// uiFS returns nil when the UI has not been embedded (dev mode / go run).
// The server handles nil staticFS by not serving static files.
func uiFS() fs.FS {
	return nil
}
