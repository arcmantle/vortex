//go:build windows

package terminal

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	defaultPTYCols = 120
	defaultPTYRows = 32
)

type windowsProcess struct {
	handle windows.Handle
	pid    int
}

func (p *windowsProcess) Wait() (int, error) {
	_, err := windows.WaitForSingleObject(p.handle, windows.INFINITE)
	if err != nil {
		return 1, err
	}

	var code uint32
	if err := windows.GetExitCodeProcess(p.handle, &code); err != nil {
		_ = windows.CloseHandle(p.handle)
		return 1, err
	}
	if err := windows.CloseHandle(p.handle); err != nil {
		return int(code), err
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
	reader  io.ReadCloser
	closers []func() error
}

type conPTYResources struct {
	hostInput     windows.Handle
	hostOutput    windows.Handle
	pseudoConsole windows.Handle

	stdin  *os.File
	stdout *os.File
}

func (s *conPTYStream) Read(p []byte) (int, error) {
	return s.reader.Read(p)
}

func (s *conPTYStream) Close() error {
	var firstErr error
	if s.reader != nil {
		err := s.reader.Close()
		if err != nil {
			firstErr = err
		}
	}
	for _, closer := range s.closers {
		err := closer()
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	s.reader = nil
	s.closers = nil
	return firstErr
}

func startChildProcess(cmd *exec.Cmd) (startedChildProcess, error) {
	setChildFlags(cmd)
	cmd.Env = ensureWindowsTerminalEnv(cmd.Environ())

	resources, err := createConPTYResources(defaultPTYCols, defaultPTYRows)
	if err != nil {
		return startedChildProcess{}, err
	}
	defer func() {
		if resources != nil {
			resources.close()
		}
	}()

	attrList, err := createPseudoConsoleAttributeList(resources.pseudoConsole)
	if err != nil {
		return startedChildProcess{}, err
	}
	defer attrList.Delete()

	processInfo, err := createConPTYProcess(cmd, attrList)
	if err != nil {
		return startedChildProcess{}, err
	}
	_ = windows.CloseHandle(processInfo.Thread)

	started := resources.buildStartedChildProcess(processInfo)
	resources = nil
	return started, nil
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
	if r.stdin != nil {
		_ = r.stdin.Close()
		r.stdin = nil
	} else if r.hostInput != 0 {
		_ = windows.CloseHandle(r.hostInput)
	}
	if r.stdout != nil {
		_ = r.stdout.Close()
		r.stdout = nil
	} else if r.hostOutput != 0 {
		_ = windows.CloseHandle(r.hostOutput)
	}
	if r.pseudoConsole != 0 {
		windows.ClosePseudoConsole(r.pseudoConsole)
	}
	r.hostInput = 0
	r.hostOutput = 0
	r.pseudoConsole = 0
}

func createPseudoConsoleAttributeList(pseudoConsole windows.Handle) (*windows.ProcThreadAttributeListContainer, error) {
	attrList, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return nil, err
	}
	if err := attrList.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, unsafe.Pointer(&pseudoConsole), unsafe.Sizeof(pseudoConsole)); err != nil {
		attrList.Delete()
		return nil, err
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

func (r *conPTYResources) buildStartedChildProcess(processInfo windows.ProcessInformation) startedChildProcess {
	r.stdin = os.NewFile(uintptr(r.hostInput), "conpty-in")
	r.stdout = os.NewFile(uintptr(r.hostOutput), "conpty-out")

	stdin := r.stdin
	stdout := r.stdout
	pseudoConsole := r.pseudoConsole

	r.hostInput = 0
	r.hostOutput = 0
	r.pseudoConsole = 0

	stream := &conPTYStream{
		reader: stdout,
		closers: []func() error{
			stdin.Close,
			func() error {
				windows.ClosePseudoConsole(pseudoConsole)
				return nil
			},
		},
	}

	return startedChildProcess{
		stream:  stream,
		process: &windowsProcess{handle: processInfo.Process, pid: int(processInfo.ProcessId)},
		input: func(data []byte) error {
			_, err := stdin.Write(data)
			return err
		},
		resize: func(cols, rows uint16) error {
			return windows.ResizePseudoConsole(pseudoConsole, windows.Coord{X: int16(cols), Y: int16(rows)})
		},
	}
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
