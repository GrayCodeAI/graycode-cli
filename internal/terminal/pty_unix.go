//go:build !windows

package terminal

import (
	"os"
	"os/exec"

	"github.com/creack/pty"
)

type ptyDevice struct {
	file *os.File
}

func startPTY(cmd *exec.Cmd, rows, cols int) (*ptyDevice, error) {
	var sz *pty.Winsize
	if rows > 0 && cols > 0 {
		sz = &pty.Winsize{
			Rows: uint16(rows),
			Cols: uint16(cols),
		}
	}

	ptmx, err := pty.StartWithSize(cmd, sz)
	if err != nil {
		return nil, err
	}
	return &ptyDevice{file: ptmx}, nil
}

func (p *ptyDevice) Resize(rows, cols int) error {
	if p == nil || p.file == nil {
		return nil
	}
	return pty.Setsize(p.file, &pty.Winsize{
		Rows: uint16(rows),
		Cols: uint16(cols),
	})
}

func (p *ptyDevice) Read(b []byte) (int, error) {
	return p.file.Read(b)
}

func (p *ptyDevice) Write(b []byte) (int, error) {
	return p.file.Write(b)
}

func (p *ptyDevice) Close() error {
	if p == nil || p.file == nil {
		return nil
	}
	return p.file.Close()
}
