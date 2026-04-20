package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"arcmantle/vortex"

	"github.com/arcmantle/rembed"
)

const (
	docsRepoOwner = "arcmantle"
	docsRepoName  = "vortex"
	docsDevRef    = "master"
)

func runDocsCommand(force, noOpen bool) error {
	baseDir, err := docsBaseDir()
	if err != nil {
		return err
	}

	version := strings.TrimSpace(Version)
	if version == "" {
		version = "dev"
	}

	// Rewrite links to embedded doc files from .md to .html so they resolve
	// as local sibling pages in the generated output directory.
	readme := string(vortex.EmbeddedREADME)
	for mdPath := range vortex.EmbeddedDocs {
		htmlPath := strings.TrimSuffix(mdPath, ".md") + ".html"
		readme = strings.ReplaceAll(readme, "("+mdPath+")", "("+htmlPath+")")
	}

	docPath, err := rembed.WriteDocsWithOptions(baseDir, readme, rembed.WriteOptions{
		Version:    version,
		Title:      "Vortex Documentation",
		SourcePath: "embedded README.md",
		Force:      force,
	})
	if err != nil {
		return fmt.Errorf("write embedded docs: %w", err)
	}

	// Write additional embedded doc pages as sibling HTML files.
	docDir := filepath.Dir(docPath)
	for mdPath, content := range vortex.EmbeddedDocs {
		htmlName := strings.TrimSuffix(mdPath, ".md") + ".html"
		htmlDest := filepath.Join(docDir, htmlName)

		if !force {
			if info, statErr := os.Stat(htmlDest); statErr == nil && info.Size() > 0 {
				continue
			}
		}

		// Derive a page title from the first markdown heading.
		title := pageTitleFromMarkdown(string(content))

		htmlBytes, renderErr := rembed.RenderHTML(string(content), rembed.WriteOptions{
			Version:    version,
			Title:      title,
			SourcePath: mdPath,
			Force:      force,
		})
		if renderErr != nil {
			return fmt.Errorf("render %s: %w", mdPath, renderErr)
		}

		if mkdirErr := os.MkdirAll(filepath.Dir(htmlDest), 0o755); mkdirErr != nil {
			return fmt.Errorf("create dir for %s: %w", htmlName, mkdirErr)
		}
		if writeErr := os.WriteFile(htmlDest, htmlBytes, 0o644); writeErr != nil {
			return fmt.Errorf("write %s: %w", htmlName, writeErr)
		}
	}

	fmt.Println("Documentation file:", docPath)
	if noOpen {
		fmt.Println("Open disabled with --no-open")
		return nil
	}

	if err := rembed.OpenInBrowser(docPath); err != nil {
		return fmt.Errorf("opening docs in browser: %w", err)
	}

	fmt.Println("Opened documentation in your default browser.")
	return nil
}

// pageTitleFromMarkdown extracts the first H1 heading from markdown content.
func pageTitleFromMarkdown(md string) string {
	for _, line := range strings.Split(md, "\n") {
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return "Vortex Documentation"
}

func docsBaseDir() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve user cache dir: %w", err)
	}
	return filepath.Join(cacheDir, "vortex"), nil
}
