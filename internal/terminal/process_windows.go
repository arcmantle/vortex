//go:build windows

package terminal

import (
	"fmt"
	"io"
	"os/exec"
	"sync"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procUpdateProcThreadAttribute = syscall.NewLazyDLL("kernel32.dll").NewProc("UpdateProcThreadAttribute")

// PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE = ProcThreadAttributeValue(22, FALSE, TRUE, FALSE)
// = 22 | (1 << 17) = 0x00020016
const procThreadAttributePseudoConsole uintptr = 0x00020016

const (
	defaultPTYCols = 120
	defaultPTYRows = 32
)

type windowsProcess struct {
	handle windows.Handle
	pid    int
}

func (p *windowsProcess) Wait() (int, error) {
	defer windows.CloseHandle(p.handle)

	_, err := windows.WaitForSingleObject(p.handle, windows.INFINITE)
	if err != nil {
		return 1, err
	}

	var code uint32
	if err := windows.GetExitCodeProcess(p.handle, &code); err != nil {
		return 1, err
	}
	if code == 0 {
		return 0, nil
	}
	return int(code), fmt.Errorf("process exited with code %d", code)
}

func (p *windowsProcess) PID() int {
	return p.pid
}

type conPTYStream struct {
	mu      sync.Mutex
	reader  io.ReadCloser
	closers []func() error
}

type conPTYResources struct {
	hostInput     windows.Handle
	hostOutput    windows.Handle
	pseudoConsole windows.Handle
}

type windowsConPTY struct {
	process       *windowsProcess
	stream        *conPTYStream
	hostInput     windows.Handle
	hostOutput    windows.Handle
	pseudoConsole windows.Handle
	closePCOnce   sync.Once
}

func (s *conPTYStream) Read(p []byte) (int, error) {
	return s.reader.Read(p)
}

func (s *conPTYStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reader == nil {
		return nil // already closed
	}
	var firstErr error
	if err := s.reader.Close(); err != nil {
		firstErr = err
	}
	s.reader = nil
	for _, closer := range s.closers {
		if err := closer(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	s.closers = nil
	return firstErr
}

func (p *windowsConPTY) Stream() io.ReadCloser { return p.stream }

func (p *windowsConPTY) Process() processHandle { return p.process }

func (p *windowsConPTY) WriteInput(data []byte) error {
	if p.hostInput == 0 {
		return nil
	}
	var n uint32
	return windows.WriteFile(p.hostInput, data, &n, nil)
}

func (p *windowsConPTY) Resize(cols, rows uint16) error {
	if p.pseudoConsole == 0 {
		return nil
	}
	return windows.ResizePseudoConsole(p.pseudoConsole, windows.Coord{X: int16(cols), Y: int16(rows)})
}

func (p *windowsConPTY) SignalEOF() {
	p.closePseudoConsole()
}

func (p *windowsConPTY) closeInput() error {
	if p.hostInput == 0 {
		return nil
	}
	err := windows.CloseHandle(p.hostInput)
	p.hostInput = 0
	return err
}

func (p *windowsConPTY) closeOutput() error {
	var err error
	if p.hostOutput != 0 {
		err = windows.CloseHandle(p.hostOutput)
		p.hostOutput = 0
	}
	p.closePseudoConsole()
	return err
}

func (p *windowsConPTY) closePseudoConsole() {
	p.closePCOnce.Do(func() {
		if p.pseudoConsole != 0 {
			windows.ClosePseudoConsole(p.pseudoConsole)
			p.pseudoConsole = 0
		}
	})
}

func startChildProcess(cmd *exec.Cmd) (childTransport, error) {
	setChildFlags(cmd)
	cmd.Env = ensureWindowsTerminalEnv(cmd.Environ())

	return newWindowsConPTY(cmd)
}

func newWindowsConPTY(cmd *exec.Cmd) (*windowsConPTY, error) {
	resources, err := createConPTYResources(defaultPTYCols, defaultPTYRows)
	if err != nil {
		return nil, err
	}
	defer func() {
		if resources != nil {
			resources.close()
		}
	}()

	attrList, err := createPseudoConsoleAttributeList(resources.pseudoConsole)
	if err != nil {
		return nil, err
	}
	defer attrList.Delete()

	pr, pw := io.Pipe()
	readerReady := make(chan struct{})
	go conPTYPipeReader(resources.hostOutput, pw, readerReady)
	<-readerReady

	processInfo, err := createConPTYProcess(cmd, attrList)
	if err != nil {
		pw.CloseWithError(err)
		return nil, err
	}
	_ = windows.CloseHandle(processInfo.Thread)

	transport := resources.buildWindowsConPTY(processInfo, pr)
	resources = nil
	return transport, nil
}

// conPTYPipeReader runs in a goroutine dedicated to reading from the ConPTY
// output pipe. It closes readerReady before the first blocking ReadFile call so
// the caller knows the I/O is pending and can safely create the child process.
// On Windows 11 24H2+, conhost closes the write end of the pipe when all
// client processes disconnect, naturally ending the read loop. For older
// Windows, the caller should call ClosePseudoConsole (via signalEOF) to close
// the write end after the process exits.
func conPTYPipeReader(h windows.Handle, pw *io.PipeWriter, readerReady chan struct{}) {
	defer pw.Close()
	buf := make([]byte, 4_096)
	first := true
	for {
		if first {
			first = false
			close(readerReady) // ReadFile is about to be issued
		}
		var n uint32
		err := windows.ReadFile(h, buf, &n, nil)
		if n > 0 {
			if _, writeErr := pw.Write(buf[:n]); writeErr != nil {
				return
			}
		}
		if err != nil {
			if err != windows.ERROR_BROKEN_PIPE && err != windows.ERROR_HANDLE_EOF {
				pw.CloseWithError(err)
			}
			return
		}
	}
}

func createConPTYResources(cols, rows int16) (*conPTYResources, error) {
	var ptyInput windows.Handle
	var hostInput windows.Handle
	if err := windows.CreatePipe(&ptyInput, &hostInput, nil, 0); err != nil {
		return nil, err
	}

	var hostOutput windows.Handle
	var ptyOutput windows.Handle
	if err := windows.CreatePipe(&hostOutput, &ptyOutput, nil, 0); err != nil {
		_ = windows.CloseHandle(ptyInput)
		_ = windows.CloseHandle(hostInput)
		return nil, err
	}

	coord := windows.Coord{X: cols, Y: rows}
	var pseudoConsole windows.Handle
	if err := windows.CreatePseudoConsole(coord, ptyInput, ptyOutput, 0, &pseudoConsole); err != nil {
		_ = windows.CloseHandle(ptyInput)
		_ = windows.CloseHandle(hostInput)
		_ = windows.CloseHandle(hostOutput)
		_ = windows.CloseHandle(ptyOutput)
		return nil, err
	}
	_ = windows.CloseHandle(ptyInput)
	_ = windows.CloseHandle(ptyOutput)

	return &conPTYResources{
		hostInput:     hostInput,
		hostOutput:    hostOutput,
		pseudoConsole: pseudoConsole,
	}, nil
}

func (r *conPTYResources) close() {
	if r == nil {
		return
	}
	if r.hostInput != 0 {
		_ = windows.CloseHandle(r.hostInput)
		r.hostInput = 0
	}
	if r.hostOutput != 0 {
		_ = windows.CloseHandle(r.hostOutput)
		r.hostOutput = 0
	}
	if r.pseudoConsole != 0 {
		windows.ClosePseudoConsole(r.pseudoConsole)
		r.pseudoConsole = 0
	}
}

func createPseudoConsoleAttributeList(pseudoConsole windows.Handle) (*windows.ProcThreadAttributeListContainer, error) {
	attrList, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return nil, err
	}
	// NOTE: windows.ProcThreadAttributeListContainer.Update() copies the value
	// to a heap buffer and passes a pointer to that buffer as lpValue. However,
	// PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE requires lpValue to be the HPCON
	// handle value itself (not a pointer to it), matching how the Win32 sample
	// code calls UpdateProcThreadAttribute(list, 0, ATTR, hPC, sizeof(HPCON)).
	// Passing a pointer causes STATUS_DLL_INIT_FAILED in the child process.
	ret, _, e := procUpdateProcThreadAttribute.Call(
		uintptr(unsafe.Pointer(attrList.List())),
		0,
		procThreadAttributePseudoConsole,
		uintptr(pseudoConsole), // handle VALUE directly, not &pseudoConsole
		unsafe.Sizeof(pseudoConsole),
		0, 0,
	)
	if ret == 0 {
		attrList.Delete()
		return nil, fmt.Errorf("UpdateProcThreadAttribute: %w", e)
	}
	return attrList, nil
}

func createConPTYProcess(cmd *exec.Cmd, attrList *windows.ProcThreadAttributeListContainer) (windows.ProcessInformation, error) {
	commandLine, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(cmd.Args))
	if err != nil {
		return windows.ProcessInformation{}, err
	}
	applicationName, err := windows.UTF16PtrFromString(cmd.Path)
	if err != nil {
		return windows.ProcessInformation{}, err
	}

	var currentDir *uint16
	if cmd.Dir != "" {
		currentDir, err = windows.UTF16PtrFromString(cmd.Dir)
		if err != nil {
			return windows.ProcessInformation{}, err
		}
	}

	envBlock, err := createWindowsEnvBlock(cmd.Env)
	if err != nil {
		return windows.ProcessInformation{}, err
	}

	startupInfo := windows.StartupInfoEx{}
	startupInfo.Cb = uint32(unsafe.Sizeof(startupInfo))
	startupInfo.Flags = windows.STARTF_USESTDHANDLES
	startupInfo.ProcThreadAttributeList = attrList.List()

	creationFlags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_UNICODE_ENVIRONMENT)
	if cmd.SysProcAttr != nil {
		creationFlags |= cmd.SysProcAttr.CreationFlags
	}

	var envPtr *uint16
	if len(envBlock) > 0 {
		envPtr = &envBlock[0]
	}

	processInfo := windows.ProcessInformation{}
	if err := windows.CreateProcess(applicationName, commandLine, nil, nil, false, creationFlags, envPtr, currentDir, &startupInfo.StartupInfo, &processInfo); err != nil {
		return windows.ProcessInformation{}, err
	}
	return processInfo, nil
}

func (r *conPTYResources) buildWindowsConPTY(processInfo windows.ProcessInformation, pr *io.PipeReader) *windowsConPTY {
	transport := &windowsConPTY{
		process:       &windowsProcess{
			handle: processInfo.Process,
			pid: int(processInfo.ProcessId),
		},
		hostInput:     r.hostInput,
		hostOutput:    r.hostOutput,
		pseudoConsole: r.pseudoConsole,
	}

	r.hostInput = 0
	r.hostOutput = 0
	r.pseudoConsole = 0

	transport.stream = &conPTYStream{
		reader: pr,
		closers: []func() error{
			transport.closeInput,
			transport.closeOutput,
		},
	}

	return transport
}

func ensureWindowsTerminalEnv(env []string) []string {
	if !hasEnvKey(env, "TERM") {
		env = append(env, "TERM=xterm-256color")
	}
	if !hasEnvKey(env, "COLORTERM") {
		env = append(env, "COLORTERM=truecolor")
	}
	return env
}

func createWindowsEnvBlock(env []string) ([]uint16, error) {
	if len(env) == 0 {
		return []uint16{0, 0}, nil
	}

	block := make([]uint16, 0, 256)
	for _, entry := range env {
		for _, r := range entry {
			if r == 0 {
				return nil, fmt.Errorf("environment entry contains NUL: %q", entry)
			}
		}
		block = append(block, utf16.Encode([]rune(entry))...)
		block = append(block, 0)
	}
	block = append(block, 0)
	return block, nil
}
