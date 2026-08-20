//go:build windows

package terminal

import (
	"errors"
	"io"
	"os/exec"
)

type ptyDevice struct {
	stdin  io.WriteCloser
	stdout io.ReadCloser
}

func startPTY(cmd *exec.Cmd, rows, cols int) (*ptyDevice, error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, err
	}

	return &ptyDevice{
		stdin:  stdin,
		stdout: stdout,
	}, nil
}

func (p *ptyDevice) Resize(rows, cols int) error {
	return nil
}

func (p *ptyDevice) Read(b []byte) (int, error) {
	if p == nil || p.stdout == nil {
		return 0, errors.New("terminal closed")
	}
	return p.stdout.Read(b)
}

func (p *ptyDevice) Write(b []byte) (int, error) {
	if p == nil || p.stdin == nil {
		return 0, errors.New("terminal closed")
	}
	return p.stdin.Write(b)
}

func (p *ptyDevice) Close() error {
	if p == nil {
		return nil
	}
	if p.stdin != nil {
		_ = p.stdin.Close()
	}
	if p.stdout != nil {
		_ = p.stdout.Close()
	}
	return nil
}
