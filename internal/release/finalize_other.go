//go:build !darwin

package release

import (
	"fmt"
	"os"
)

// FinalizeInstall sets the executable bit on a freshly-installed binary.
func FinalizeInstall(path string) error {
	if err := os.Chmod(path, 0o755); err != nil {
		return fmt.Errorf("set installed binary executable: %w", err)
	}
	return nil
}
