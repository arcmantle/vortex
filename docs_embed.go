package vortex

import _ "embed"

//go:embed README.md
var EmbeddedREADME []byte

//go:embed docs/runtimes-javascript.md
var EmbeddedDocsJavaScript []byte

//go:embed docs/runtimes-csharp.md
var EmbeddedDocsCSharp []byte

//go:embed docs/runtimes-go.md
var EmbeddedDocsGo []byte

// EmbeddedDocs maps repository-relative paths to their embedded content.
// Paths match the link targets used in README.md.
var EmbeddedDocs = map[string][]byte{
	"docs/runtimes-javascript.md": EmbeddedDocsJavaScript,
	"docs/runtimes-csharp.md":     EmbeddedDocsCSharp,
	"docs/runtimes-go.md":         EmbeddedDocsGo,
}
