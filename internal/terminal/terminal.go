// Package terminal manages terminals — persistent output containers that host
// transient child processes. Each terminal accumulates raw output chunks in a
// bounded buffer and supports live subscriptions via channels.
package terminal

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	// maxBufferedChunks bounds the number of buffered writes retained per terminal.
	maxBufferedChunks = 4_096
	// maxBufferedBytes bounds the total retained terminal output.
	maxBufferedBytes = 4 << 20
)

// OutputChunk is a timestamped slice of terminal output bytes.
type OutputChunk struct {
	T    time.Time
	Data []byte
}

type outputBuffer struct {
	chunks     []OutputChunk
	totalBytes int
}

func (b *outputBuffer) add(chunk OutputChunk) {
	b.chunks = append(b.chunks, chunk)
	b.totalBytes += len(chunk.Data)
	for len(b.chunks) > maxBufferedChunks || b.totalBytes > maxBufferedBytes {
		b.totalBytes -= len(b.chunks[0].Data)
		copy(b.chunks, b.chunks[1:])
		last := len(b.chunks) - 1
		b.chunks[last] = OutputChunk{}
		b.chunks = b.chunks[:last]
	}
}

func (b *outputBuffer) snapshot() []OutputChunk {
	out := make([]OutputChunk, len(b.chunks))
	copy(out, b.chunks)
	return out
}

func (b *outputBuffer) clear() {
	for i := range b.chunks {
		b.chunks[i] = OutputChunk{}
	}
	b.chunks = nil
	b.totalBytes = 0
}

type resizeFunc func(cols, rows uint16) error
type inputFunc func([]byte) error

type processHandle interface {
	Wait() (int, error)
	PID() int
}

type startedChildProcess struct {
	stream  io.ReadCloser
	process processHandle
	input   inputFunc
	resize  resizeFunc
}

type execProcess struct {
	cmd *exec.Cmd
}

func hasEnvKey(env []string, key string) bool {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return true
		}
	}
	return false
}

func (p *execProcess) Wait() (int, error) {
	err := p.cmd.Wait()
	if p.cmd.ProcessState == nil {
		if err != nil {
			return 1, err
		}
		return 0, nil
	}
	code := p.cmd.ProcessState.ExitCode()
	if code < 0 {
		code = 1
	}
	return code, err
}

func (p *execProcess) PID() int {
	if p.cmd == nil || p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

// Terminal wraps a running child process and accumulates its output in a
// persistent buffer that survives process restarts.
type Terminal struct {
	ID      string
	Label   string
	Command string
	Args    []string

	mu        sync.RWMutex
	outputBuf outputBuffer
	subs      []chan OutputChunk
	done      chan struct{}
	exited    bool // set under mu when drain finishes
	exitErr   error
	exitCode  int
	cancel    context.CancelFunc // cancels the child-process context
	process   processHandle
	input     inputFunc
	resize    resizeFunc
}

// New creates and starts a Terminal with a child process. The process runs
// until its command exits or ctx is cancelled.
func New(ctx context.Context, id, label, command string, args []string, dir string) (*Terminal, error) {
	ctx, cancel := context.WithCancel(ctx)
	t := &Terminal{
		ID:      id,
		Label:   label,
		Command: command,
		Args:    args,
		done:    make(chan struct{}),
		cancel:  cancel,
	}

	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = dir
	started, err := startChildProcess(cmd)
	if err != nil {
		return nil, err
	}
	t.process = started.process
	t.input = started.input
	t.resize = started.resize

	go func() {
		select {
		case <-ctx.Done():
			if pid := t.PID(); pid > 0 {
				_ = killProcessTree(pid)
			}
		case <-t.done:
		}
	}()

	go t.drain(started.stream, started.process)
	return t, nil
}

// WriteInput sends raw bytes to the child process stdin when supported.
func (t *Terminal) WriteInput(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	t.mu.RLock()
	input := t.input
	exited := t.exited
	t.mu.RUnlock()
	if exited || input == nil {
		return nil
	}
	return input(data)
}

func (t *Terminal) drain(r io.ReadCloser, proc processHandle) {
	defer r.Close()

	buf := make([]byte, 4_096)
	for {
		n, readErr := r.Read(buf)
		if n > 0 {
			t.appendChunk(time.Now(), buf[:n])
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				t.appendChunk(time.Now(), []byte("\r\n\x1b[31m[stream error: "+readErr.Error()+"]\x1b[0m\r\n"))
			}
			break
		}
	}

	code, err := proc.Wait()
	t.mu.Lock()
	exitChunk := OutputChunk{T: time.Now(), Data: []byte("\r\n\x1b[2m[process exited]\x1b[0m\r\n")}
	t.outputBuf.add(exitChunk)
	for _, ch := range t.subs {
		select {
		case ch <- exitChunk:
		default:
		}
	}
	t.exited = true
	t.exitErr = err
	t.exitCode = code
	for _, ch := range t.subs {
		close(ch)
	}
	t.subs = nil
	t.mu.Unlock()
	close(t.done)
}

func (t *Terminal) appendChunk(ts time.Time, data []byte) {
	if len(data) == 0 {
		return
	}

	chunk := OutputChunk{T: ts, Data: append([]byte(nil), data...)}
	t.mu.Lock()
	t.outputBuf.add(chunk)
	for _, ch := range t.subs {
		select {
		case ch <- chunk:
		default:
		}
	}
	t.mu.Unlock()
}

// OutputAndSubscribe atomically returns a snapshot of all buffered output and
// registers a live subscription. Using both under the same lock prevents the
// race where output produced between separate snapshot and subscribe calls
// would be silently dropped.
func (t *Terminal) OutputAndSubscribe() ([]OutputChunk, <-chan OutputChunk, func()) {
	ch := make(chan OutputChunk, 256)
	t.mu.Lock()
	snapshot := t.outputBuf.snapshot()
	if t.exited {
		t.mu.Unlock()
		close(ch)
		return snapshot, ch, func() {}
	}
	t.subs = append(t.subs, ch)
	t.mu.Unlock()

	cancel := func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		for i, s := range t.subs {
			if s == ch {
				t.subs = append(t.subs[:i], t.subs[i+1:]...)
				for len(ch) > 0 {
					<-ch
				}
				close(ch)
				return
			}
		}
	}
	return snapshot, ch, cancel
}

// Output returns a snapshot of the buffered output.
func (t *Terminal) Output() []OutputChunk {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.outputBuf.snapshot()
}

// Subscribe registers a live subscription for new output without replaying the
// existing buffer. Useful when the caller already has the history (e.g. after
// a restart on the same SSE connection).
func (t *Terminal) Subscribe() (<-chan OutputChunk, func()) {
	ch := make(chan OutputChunk, 256)
	t.mu.Lock()
	if t.exited {
		t.mu.Unlock()
		close(ch)
		return ch, func() {}
	}
	t.subs = append(t.subs, ch)
	t.mu.Unlock()

	cancel := func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		for i, s := range t.subs {
			if s == ch {
				t.subs = append(t.subs[:i], t.subs[i+1:]...)
				for len(ch) > 0 {
					<-ch
				}
				close(ch)
				return
			}
		}
	}
	return ch, cancel
}

// Resize updates the child terminal dimensions when the underlying transport
// supports it.
func (t *Terminal) Resize(cols, rows uint16) error {
	if cols == 0 || rows == 0 {
		return nil
	}

	t.mu.RLock()
	resize := t.resize
	t.mu.RUnlock()
	if resize == nil {
		return nil
	}
	return resize(cols, rows)
}

// Done returns a channel closed when the process has exited.
func (t *Terminal) Done() <-chan struct{} { return t.done }

// ExitErr returns the error from cmd.Wait(), or nil if still running.
func (t *Terminal) ExitErr() error {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.exitErr
}

// ExitCode returns the process exit code, or 0 if still running.
func (t *Terminal) ExitCode() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.exitCode
}

// PID returns the child process ID, or 0 if the process was never started.
func (t *Terminal) PID() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.exited || t.process == nil {
		return 0
	}
	return t.process.PID()
}

// ClearBuffer resets the output ring buffer.
func (t *Terminal) ClearBuffer() {
	t.mu.Lock()
	t.outputBuf.clear()
	t.mu.Unlock()
}

// Kill terminates the child process tree. It is safe to call multiple times.
func (t *Terminal) Kill() {
	if pid := t.PID(); pid > 0 {
		killProcessTree(pid)
	}
	if t.cancel != nil {
		t.cancel()
	}
}

// KillProcessTreeByPID terminates a process and its children by PID.
func KillProcessTreeByPID(pid int) error {
	if pid <= 0 {
		return nil
	}
	return killProcessTree(pid)
}

// seedFrom copies the output history from an old terminal into this one so
// the UI preserves history across process restarts.
func (t *Terminal) seedFrom(old *Terminal) {
	old.mu.RLock()
	chunks := old.outputBuf.snapshot()
	old.mu.RUnlock()

	t.mu.Lock()
	for _, chunk := range chunks {
		t.outputBuf.add(chunk)
	}
	t.mu.Unlock()
}

// Manager holds a set of terminals.
type Manager struct {
	mu        sync.RWMutex
	terminals map[string]*Terminal
	order     []string // insertion order
}

func NewManager() *Manager {
	return &Manager{terminals: make(map[string]*Terminal)}
}

// Start launches a new process in a terminal and registers it. If a terminal
// with the same ID already exists, the new one inherits the old output buffer
// so the UI preserves history.
func (m *Manager) Start(ctx context.Context, id, label, command string, args []string, dir string) (*Terminal, error) {
	t, err := New(ctx, id, label, command, args, dir)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	if old, ok := m.terminals[id]; ok {
		t.seedFrom(old)
	} else {
		m.order = append(m.order, id)
	}
	m.terminals[id] = t
	m.mu.Unlock()
	return t, nil
}

// Get returns a terminal by ID.
func (m *Manager) Get(id string) (*Terminal, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.terminals[id]
	return t, ok
}

// All returns all registered terminals in insertion order.
func (m *Manager) All() []*Terminal {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Terminal, 0, len(m.order))
	for _, id := range m.order {
		if t, ok := m.terminals[id]; ok {
			out = append(out, t)
		}
	}
	return out
}

// Prune removes terminal entries whose IDs are not present in keepIDs.
// It does not kill processes; callers must stop any removed jobs first.
func (m *Manager) Prune(keepIDs map[string]struct{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	newOrder := make([]string, 0, len(m.order))
	for _, id := range m.order {
		if _, keep := keepIDs[id]; keep {
			newOrder = append(newOrder, id)
			continue
		}
		delete(m.terminals, id)
	}
	m.order = newOrder
}
