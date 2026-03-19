//go:build !darwin

package upgrade

import (
	"fmt"
	"os"
)

func finalizeUnixInstall(path string) error {
	if err := os.Chmod(path, 0o755); err != nil {
		return fmt.Errorf("set installed binary executable: %w", err)
	}
	return nil
}
