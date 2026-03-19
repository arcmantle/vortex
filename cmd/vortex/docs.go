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

	ref := docsDevRef
	if strings.HasPrefix(version, "v") {
		ref = version
	}

	docPath, err := rembed.WriteDocsWithOptions(baseDir, string(vortex.EmbeddedREADME), rembed.WriteOptions{
		Version:     version,
		Title:       "Vortex Documentation",
		SourcePath:  "embedded README.md",
		Force:       force,
		LinkBaseURL: rembed.GitHubRawBaseURL(docsRepoOwner, docsRepoName, ref),
	})
	if err != nil {
		return fmt.Errorf("write embedded docs: %w", err)
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

func docsBaseDir() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve user cache dir: %w", err)
	}
	return filepath.Join(cacheDir, "vortex"), nil
}
