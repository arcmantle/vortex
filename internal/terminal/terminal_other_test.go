//go:build !windows

package terminal

import (
	"bytes"
	"context"
	"testing"
	"time"
)

func TestNewStartsChildInPTY(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	term, err := New(ctx, "tty-check", "TTY Check", "/bin/sh", []string{"-c", `test -t 1 && printf '%s' "$TERM"`})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	select {
	case <-term.Done():
	case <-ctx.Done():
		t.Fatal("timed out waiting for terminal to exit")
	}

	var output []byte
	for _, chunk := range term.Output() {
		output = append(output, chunk.Data...)
	}

	if !bytes.Contains(output, []byte("xterm-256color")) {
		t.Fatalf("terminal output %q does not show TERM from a TTY-backed process", string(output))
	}
	if code := term.ExitCode(); code != 0 {
		t.Fatalf("unexpected exit code %d with output %q", code, string(output))
	}
}
