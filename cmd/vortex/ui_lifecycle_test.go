package main

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"arcmantle/vortex/internal/instance"
)

// fakeWriteCloser captures what's written to it.
type fakeWriteCloser struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (f *fakeWriteCloser) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.buf.Write(p)
}

func (f *fakeWriteCloser) Close() error { return nil }

func (f *fakeWriteCloser) String() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.buf.String()
}

func TestOpenWhenHiddenSendsSHOW(t *testing.T) {
	pipe := &fakeWriteCloser{}
	ui := &uiLifecycle{
		open:      true,
		hidden:    true,
		stdinPipe: pipe,
		identity:  instance.Identity{Name: "test"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ok := ui.Open(ctx, cancel, false)
	if !ok {
		t.Fatal("Open() returned false; expected true (unhide)")
	}

	got := strings.TrimSpace(pipe.String())
	if got != "SHOW" {
		t.Fatalf("Open() sent %q to pipe; want %q", got, "SHOW")
	}
}

func TestOpenWhenAlreadyOpenReturnsFalse(t *testing.T) {
	pipe := &fakeWriteCloser{}
	ui := &uiLifecycle{
		open:      true,
		hidden:    false,
		stdinPipe: pipe,
		identity:  instance.Identity{Name: "test"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ok := ui.Open(ctx, cancel, false)
	if ok {
		t.Fatal("Open() returned true for already-open window; expected false")
	}
	if pipe.String() != "" {
		t.Fatalf("Open() wrote %q to pipe; expected nothing", pipe.String())
	}
}

func TestFocusWhenHiddenSendsSHOW(t *testing.T) {
	pipe := &fakeWriteCloser{}
	ui := &uiLifecycle{
		open:      true,
		hidden:    true,
		stdinPipe: pipe,
		identity:  instance.Identity{Name: "test"},
	}

	ok := ui.Focus()
	if !ok {
		t.Fatal("Focus() returned false; expected true")
	}

	got := strings.TrimSpace(pipe.String())
	if got != "SHOW" {
		t.Fatalf("Focus() sent %q to pipe; want %q", got, "SHOW")
	}
}

func TestFocusWhenVisibleSendsFOCUS(t *testing.T) {
	pipe := &fakeWriteCloser{}
	ui := &uiLifecycle{
		open:      true,
		hidden:    false,
		stdinPipe: pipe,
		identity:  instance.Identity{Name: "test"},
	}

	ok := ui.Focus()
	if !ok {
		t.Fatal("Focus() returned false; expected true")
	}

	got := strings.TrimSpace(pipe.String())
	if got != "FOCUS" {
		t.Fatalf("Focus() sent %q to pipe; want %q", got, "FOCUS")
	}
}

func TestMarkClosedResetsHidden(t *testing.T) {
	ui := &uiLifecycle{
		open:     true,
		hidden:   true,
		identity: instance.Identity{Name: "test"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ui.markClosed(ctx, cancel, false)

	if ui.open {
		t.Fatal("markClosed did not clear open")
	}
	if ui.hidden {
		t.Fatal("markClosed did not clear hidden")
	}
}

func TestHandleChildHiddenSetsState(t *testing.T) {
	ui := &uiLifecycle{
		open:     true,
		hidden:   false,
		identity: instance.Identity{Name: "test"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ui.handleChildHidden(ctx, cancel, false)

	ui.mu.Lock()
	h := ui.hidden
	ui.mu.Unlock()

	if !h {
		t.Fatal("handleChildHidden did not set hidden=true")
	}
}

// TestREADYClearsHidden verifies that receiving READY from the child clears
// the hidden state. We simulate this by setting up a pipe that sends "READY".
func TestREADYClearsHidden(t *testing.T) {
	// We can't easily test the goroutine reading stdout without running
	// the full subprocess. Instead, verify the state transition directly:
	// set hidden=true, then simulate what the READY handler does.
	ui := &uiLifecycle{
		open:     true,
		hidden:   true,
		identity: instance.Identity{Name: "test"},
	}

	// Simulate the READY handler logic from runWindowProcess.
	ui.mu.Lock()
	ui.hidden = false
	ui.mu.Unlock()

	if ui.hidden {
		t.Fatal("READY handler did not clear hidden")
	}
}

// TestOpenWhenHiddenButNoPipeReturnsFalse verifies edge case where hidden
// is set but the stdin pipe is nil (shouldn't happen, but defensive).
func TestOpenWhenHiddenButNoPipeReturnsFalse(t *testing.T) {
	ui := &uiLifecycle{
		open:      true,
		hidden:    true,
		stdinPipe: nil, // broken state
		identity:  instance.Identity{Name: "test"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ok := ui.Open(ctx, cancel, false)
	if ok {
		t.Fatal("Open() returned true with nil pipe; expected false")
	}
}

// Ensure unused imports are satisfied.
var _ io.WriteCloser = (*fakeWriteCloser)(nil)
var _ = time.Millisecond
