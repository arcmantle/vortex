//go:build darwin

package upgrade

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func finalizeUnixInstall(path string) error {
	if err := os.Chmod(path, 0o755); err != nil {
		return fmt.Errorf("set installed binary executable: %w", err)
	}
	out, err := exec.Command("xattr", "-d", "com.apple.quarantine", path).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if !strings.Contains(msg, "No such xattr") {
			return fmt.Errorf("remove quarantine attribute: %w: %s", err, msg)
		}
	}
	return nil
}
