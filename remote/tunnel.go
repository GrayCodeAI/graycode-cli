package remote

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"time"
)

// SSHTunnel wraps an SSH port-forward for remote bridge connections.
type SSHTunnel struct {
	localPort  int
	remoteHost string
	remotePort int
	user       string
	keyPath    string
	cmd        *exec.Cmd
	listener   net.Listener
}

// NewSSHTunnel creates a new SSH tunnel configuration.
func NewSSHTunnel(host string, port int, user string, auth Auth) *SSHTunnel {
	return &SSHTunnel{
		remoteHost: host,
		remotePort: port,
		user:       user,
		keyPath:    auth.KeyPath,
	}
}

// Open establishes the SSH tunnel and returns the local port.
func (t *SSHTunnel) Open(ctx context.Context) (int, error) {
	// Allocate a free local port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("allocating local port: %w", err)
	}
	t.localPort = listener.Addr().(*net.TCPAddr).Port
	t.listener = listener
	listener.Close() // release so ssh can bind to it

	// Build SSH command
	remote := fmt.Sprintf("%s@%s", t.user, t.remoteHost)
	forward := fmt.Sprintf("%d:localhost:%d", t.localPort, t.remotePort)

	args := []string{
		"-N", "-L", forward,
		"-o", "StrictHostKeyChecking=no",
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=3",
	}
	if t.keyPath != "" {
		args = append(args, "-i", t.keyPath)
	}
	args = append(args, remote)

	t.cmd = exec.CommandContext(ctx, "ssh", args...)
	t.cmd.Stderr = os.Stderr

	if err := t.cmd.Start(); err != nil {
		return 0, fmt.Errorf("starting SSH tunnel: %w", err)
	}

	// Wait briefly for tunnel to establish
	if err := waitForPort(ctx, t.localPort, 10*time.Second); err != nil {
		t.Close()
		return 0, fmt.Errorf("tunnel failed to establish: %w", err)
	}

	return t.localPort, nil
}

// LocalPort returns the local port of the tunnel.
func (t *SSHTunnel) LocalPort() int {
	return t.localPort
}

// LocalURL returns the local URL for connecting through the tunnel.
func (t *SSHTunnel) LocalURL() string {
	return "http://localhost:" + strconv.Itoa(t.localPort)
}

// Close terminates the SSH tunnel.
func (t *SSHTunnel) Close() error {
	if t.cmd != nil && t.cmd.Process != nil {
		return t.cmd.Process.Kill()
	}
	return nil
}

// waitForPort polls until a TCP port accepts connections or timeout.
func waitForPort(ctx context.Context, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for port %d", port)
}
