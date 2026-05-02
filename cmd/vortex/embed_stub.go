//go:build no_embed_ui

package main

import "io/fs"

// uiFS returns nil when the UI has been excluded via the no_embed_ui tag.
// The server handles nil staticFS by not serving static files.
func uiFS() fs.FS {
	return nil
}
