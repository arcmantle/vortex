//go:build !windows

package terminal

import (
	"os/exec"

	"github.com/creack/pty"
)

const (
	defaultPTYCols = 120
	defaultPTYRows = 32
)

func startChildProcess(cmd *exec.Cmd) (startedChildProcess, error) {
	setChildFlags(cmd)
	cmd.Env = ensureTerminalEnv(cmd.Environ())
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: defaultPTYCols, Rows: defaultPTYRows})
	if err != nil {
		return startedChildProcess{}, err
	}

	resize := func(cols, rows uint16) error {
		return pty.Setsize(ptmx, &pty.Winsize{Cols: cols, Rows: rows})
	}
	return startedChildProcess{
		stream:  ptmx,
		process: &execProcess{cmd: cmd},
		input: func(data []byte) error {
			_, err := ptmx.Write(data)
			return err
		},
		resize: resize,
	}, nil
}

func ensureTerminalEnv(env []string) []string {
	if !hasEnvKey(env, "TERM") {
		env = append(env, "TERM=xterm-256color")
	}
	if !hasEnvKey(env, "COLORTERM") {
		env = append(env, "COLORTERM=truecolor")
	}
	return env
}
