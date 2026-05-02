//go:build !no_embed_ui

package main

import (
	"embed"
	"io/fs"
	"log"
)

//go:embed all:web/dist
var embeddedUI embed.FS

func uiFS() fs.FS {
	sub, err := fs.Sub(embeddedUI, "web/dist")
	if err != nil {
		log.Fatalf("embed sub: %v", err)
	}
	return sub
}
