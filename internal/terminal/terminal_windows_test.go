//go:build windows

package terminal

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestNewCapturesOutputFromFastExitWindowsProcess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	term, err := New(ctx, "win-fast-exit", "Windows Fast Exit", "cmd.exe", []string{"/C", "echo hello from windows terminal"}, "")
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

	if !bytes.Contains(output, []byte("hello from windows terminal")) {
		t.Fatalf("terminal output %q does not contain child process output", string(output))
	}
	if code := term.ExitCode(); code != 0 {
		t.Fatalf("unexpected exit code %d with output %q", code, string(output))
	}
	if err := term.ExitErr(); err != nil {
		t.Fatalf("unexpected exit error %v with output %q", err, string(output))
	}
}

func TestStartChildProcessCapturesOutputFromFastExitWindowsProcess(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/C", "echo hello from low level windows terminal")

	started, err := startChildProcess(cmd)
	if err != nil {
		t.Fatalf("startChildProcess() error = %v", err)
	}
	proc := started.Process()

	waitDone := make(chan struct{})
	var exitErr error
	go func() {
		defer close(waitDone)
		_, exitErr = proc.Wait()
		started.SignalEOF()
	}()

	output, readErr := io.ReadAll(started.Stream())
	<-waitDone
	_ = started.Stream().Close()

	if readErr != nil {
		t.Fatalf("io.ReadAll() error = %v", readErr)
	}
	if exitErr != nil {
		t.Fatalf("unexpected exit error %v with output %q", exitErr, string(output))
	}
	if !bytes.Contains(output, []byte("hello from low level windows terminal")) {
		t.Fatalf("low-level output %q does not contain child process output", string(output))
	}
}

func TestNewWritesInputAndResizesInteractiveWindowsProcess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	term, err := New(ctx, "win-interactive", "Windows Interactive", "cmd.exe", nil, "")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := term.Resize(100, 30); err != nil {
		t.Fatalf("Resize() error = %v", err)
	}
	if err := term.WriteInput([]byte("echo hello from interactive windows terminal\r\nexit\r\n")); err != nil {
		t.Fatalf("WriteInput() error = %v", err)
	}

	select {
	case <-term.Done():
	case <-ctx.Done():
		t.Fatal("timed out waiting for interactive terminal to exit")
	}

	var output []byte
	for _, chunk := range term.Output() {
		output = append(output, chunk.Data...)
	}

	if !bytes.Contains(output, []byte("hello from interactive windows terminal")) {
		t.Fatalf("interactive output %q does not contain echoed input", string(output))
	}
}

func TestKillStopsInteractiveWindowsProcess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	term, err := New(ctx, "win-kill", "Windows Kill", "cmd.exe", nil, "")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	term.Kill()

	select {
	case <-term.Done():
	case <-ctx.Done():
		t.Fatal("timed out waiting for killed terminal to exit")
	}
}

func TestNewCapturesRichStreamingWindowsProcess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(testFile), "..", ".."))
	goExe, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("exec.LookPath(go) error = %v", err)
	}

	term, err := New(ctx, "win-rich-stream", "Windows Rich Stream", goExe, []string{"run", filepath.Join(repoRoot, "mock", "terminal-smoke")}, repoRoot)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	select {
	case <-term.Done():
	case <-ctx.Done():
		t.Fatal("timed out waiting for rich streaming terminal to exit")
	}

	var output []byte
	for _, chunk := range term.Output() {
		output = append(output, chunk.Data...)
	}

	if !bytes.Contains(output, []byte("terminal smoke starting")) {
		t.Fatalf("rich streaming output %q does not contain the initial smoke banner", string(output))
	}
	if !bytes.Contains(output, []byte("download complete")) {
		t.Fatalf("rich streaming output %q does not contain the completed progress line", string(output))
	}
	if !bytes.Contains(output, []byte("terminal smoke finished")) {
		t.Fatalf("rich streaming output %q does not contain the final smoke line", string(output))
	}
	if code := term.ExitCode(); code != 0 {
		t.Fatalf("unexpected exit code %d with output %q", code, string(output))
	}
	if err := term.ExitErr(); err != nil {
		t.Fatalf("unexpected exit error %v with output %q", err, string(output))
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "mock", "terminal-smoke", "main.go")); err != nil {
		t.Fatalf("expected smoke fixture to exist: %v", err)
	}
}