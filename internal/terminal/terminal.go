// Package terminal manages terminals — persistent output containers that host
// transient child processes. Each terminal accumulates output lines in a ring
// buffer and supports live subscriptions via channels.
package terminal

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os/exec"
	"sync"
	"time"
)

// maxBufferedLines is the maximum number of output lines kept per terminal.
// Older lines are discarded as new ones arrive.
const maxBufferedLines = 10_000

// Line is a single line of output.
type Line struct {
	T    time.Time
	Text string
}

// ringLine is a fixed-capacity circular buffer of Lines.
type ringLine struct {
	data []Line
	head int // index of oldest element
	size int
}

func newRingLine(cap int) ringLine {
	return ringLine{data: make([]Line, cap)}
}

func (r *ringLine) add(l Line) {
	cap := len(r.data)
	if r.size < cap {
		r.data[(r.head+r.size)%cap] = l
		r.size++
	} else {
		// buffer full: overwrite the oldest entry
		r.data[r.head] = l
		r.head = (r.head + 1) % cap
	}
}

func (r *ringLine) snapshot() []Line {
	out := make([]Line, r.size)
	cap := len(r.data)
	for i := 0; i < r.size; i++ {
		out[i] = r.data[(r.head+i)%cap]
	}
	return out
}

// Terminal wraps a running child process and accumulates its output in a
// persistent buffer that survives process restarts.
type Terminal struct {
	ID      string
	Label   string
	Command string
	Args    []string

	mu       sync.RWMutex
	lineBuf  ringLine
	subs     []chan Line
	done     chan struct{}
	exited   bool // set under mu when drain finishes
	exitErr  error
	exitCode int
	cancel   context.CancelFunc // cancels the child-process context
	cmd      *exec.Cmd          // kept for process-tree killing
}

// New creates and starts a Terminal with a child process. The process runs
// until its command exits or ctx is cancelled.
func New(ctx context.Context, id, label, command string, args []string) (*Terminal, error) {
	ctx, cancel := context.WithCancel(ctx)
	t := &Terminal{
		ID:      id,
		Label:   label,
		Command: command,
		Args:    args,
		lineBuf: newRingLine(maxBufferedLines),
		done:    make(chan struct{}),
		cancel:  cancel,
	}

	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			return killProcessTree(cmd.Process.Pid)
		}
		return nil
	}
	t.cmd = cmd
	setChildFlags(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	go t.drain(io.MultiReader(stdout, stderr), cmd)
	return t, nil
}

func (t *Terminal) drain(r io.Reader, cmd *exec.Cmd) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := Line{T: time.Now(), Text: sc.Text()}
		t.mu.Lock()
		t.lineBuf.add(line)
		for _, ch := range t.subs {
			select {
			case ch <- line:
			default:
			}
		}
		t.mu.Unlock()
	}
	err := cmd.Wait()
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			code = exitErr.ExitCode()
		} else {
			code = 1
		}
	}
	t.mu.Lock()
	t.lineBuf.add(Line{T: time.Now(), Text: ""})
	t.lineBuf.add(Line{T: time.Now(), Text: "\x1b[2m[process exited]\x1b[0m"})
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

// LinesAndSubscribe atomically returns a snapshot of all buffered lines and
// registers a live subscription. Using both under the same lock prevents the
// race where lines produced between a separate Lines() call and a separate
// Subscribe() call would be silently dropped.
func (t *Terminal) LinesAndSubscribe() ([]Line, <-chan Line, func()) {
	ch := make(chan Line, 256)
	t.mu.Lock()
	snapshot := t.lineBuf.snapshot()
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

// Lines returns a snapshot of the buffered output lines (up to maxBufferedLines).
func (t *Terminal) Lines() []Line {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.lineBuf.snapshot()
}

// Subscribe registers a live subscription for new lines without replaying
// the existing buffer. Useful when the caller already has the history (e.g.
// after a restart on the same SSE connection).
func (t *Terminal) Subscribe() (<-chan Line, func()) {
	ch := make(chan Line, 256)
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
	if t.exited || t.cmd == nil || t.cmd.Process == nil {
		return 0
	}
	return t.cmd.Process.Pid
}

// ClearBuffer resets the output ring buffer.
func (t *Terminal) ClearBuffer() {
	t.mu.Lock()
	t.lineBuf = newRingLine(maxBufferedLines)
	t.mu.Unlock()
}

// Kill terminates the child process tree. It is safe to call multiple times.
func (t *Terminal) Kill() {
	if t.cmd != nil && t.cmd.Process != nil {
		killProcessTree(t.cmd.Process.Pid)
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
	lines := old.lineBuf.snapshot()
	old.mu.RUnlock()

	t.mu.Lock()
	for _, l := range lines {
		t.lineBuf.add(l)
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
func (m *Manager) Start(ctx context.Context, id, label, command string, args []string) (*Terminal, error) {
	t, err := New(ctx, id, label, command, args)
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
