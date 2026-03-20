package webview

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
)

func openExternalURL(target string) error {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(target))
	if err != nil {
		return fmt.Errorf("parse external url: %w", err)
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("unsupported external url scheme %q", parsed.Scheme)
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", parsed.String())
	case "linux":
		cmd = exec.Command("xdg-open", parsed.String())
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", parsed.String())
	default:
		return fmt.Errorf("external browser is unsupported on %s", runtime.GOOS)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open external browser: %w", err)
	}
	return nil
}
