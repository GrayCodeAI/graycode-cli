// Package container provides Docker container lifecycle management for hawk's
// sandboxed execution environments. It wraps the Docker CLI to start, stop,
// inspect, and rebuild containers.
package container

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Error codes returned by ContainerError.
const (
	CodeNotFound   = "not_found"
	CodeNotRunning = "not_running"
	CodeExecFailed = "exec_failed"
	CodeTimeout    = "timeout"
)

// ContainerError is a typed error with a machine-readable Code field.
type ContainerError struct {
	Code    string
	Message string
}

func (e *ContainerError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// State constants returned by Status.
const (
	StateRunning  = "running"
	StateStopped  = "exited"
	StateNotFound = "not_found"
)

// dockerCmd is a function variable for exec.CommandContext, replaceable in tests.
var dockerCmd = exec.CommandContext

// Status queries Docker for the current state of a container.
// Returns StateRunning, StateStopped, or StateNotFound.
func Status(ctx context.Context, containerID string) (string, error) {
	if containerID == "" {
		return StateNotFound, nil
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := dockerCmd(ctx, "docker", "inspect", "--format", "{{.State.Status}}", containerID)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// If inspect fails, the container doesn't exist.
		errMsg := strings.TrimSpace(stderr.String())
		if strings.Contains(errMsg, "No such object") || strings.Contains(errMsg, "not found") {
			return StateNotFound, nil
		}
		return StateNotFound, &ContainerError{
			Code:    CodeNotFound,
			Message: fmt.Sprintf("docker inspect: %s", errMsg),
		}
	}

	state := strings.TrimSpace(stdout.String())
	switch state {
	case "running":
		return StateRunning, nil
	case "exited", "dead", "created":
		return StateStopped, nil
	default:
		return state, nil
	}
}

// ExecResult holds the output of a command executed in a container.
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// ExecWithStdin runs a command inside a container with stdin piped from the
// provided byte slice. Arguments are passed directly to docker exec without
// shell wrapping, making it safe for structured input (e.g. JSON).
func ExecWithStdin(ctx context.Context, containerID string, cmd []string, stdin []byte) (*ExecResult, error) {
	if containerID == "" {
		return nil, &ContainerError{Code: CodeNotFound, Message: "no container ID"}
	}

	args := append([]string{"exec", "-i", containerID}, cmd...)
	c := dockerCmd(ctx, "docker", args...)
	c.Stdin = bytes.NewReader(stdin)

	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr

	err := c.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, &ContainerError{Code: CodeTimeout, Message: "command timed out"}
	}

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, &ContainerError{
				Code:    CodeExecFailed,
				Message: fmt.Sprintf("docker exec: %v", err),
			}
		}
	}

	return &ExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	}, nil
}

// Rebuild stops and removes an existing container, then creates a new one
// from the specified image. The new container is started with sleep infinity
// and the given working directory mounted.
func Rebuild(ctx context.Context, containerID, newImage, workDir string) (string, error) {
	// Stop and remove old container if it exists.
	if containerID != "" {
		rmCtx, rmCancel := context.WithTimeout(ctx, 10*time.Second)
		defer rmCancel()
		rm := dockerCmd(rmCtx, "docker", "rm", "-f", containerID)
		_ = rm.Run()
	}

	// Create new container.
	createCtx, createCancel := context.WithTimeout(ctx, 120*time.Second)
	defer createCancel()

	args := []string{
		"run", "-d",
		"-v", workDir + ":" + workDir,
		"-w", workDir,
		newImage,
		"sleep", "infinity",
	}
	c := dockerCmd(createCtx, "docker", args...)
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr

	if err := c.Run(); err != nil {
		return "", &ContainerError{
			Code:    CodeExecFailed,
			Message: fmt.Sprintf("docker run: %s", strings.TrimSpace(stderr.String())),
		}
	}

	return strings.TrimSpace(stdout.String()), nil
}

// EnsureRunning guarantees that the specified container is in a running state.
// If the container is stopped, it starts it. If it does not exist, it creates
// a new one from the given image with the working directory mounted.
// Returns the container ID (which may differ from input if recreated).
func EnsureRunning(ctx context.Context, containerID, image, workDir string) (string, error) {
	state, err := Status(ctx, containerID)
	if err != nil {
		// If we can't determine state, try to create fresh.
		return Rebuild(ctx, "", image, workDir)
	}

	switch state {
	case StateRunning:
		return containerID, nil

	case StateStopped:
		// Try to start the existing container.
		startCtx, startCancel := context.WithTimeout(ctx, 30*time.Second)
		defer startCancel()
		start := dockerCmd(startCtx, "docker", "start", containerID)
		var stderr bytes.Buffer
		start.Stderr = &stderr
		if err := start.Run(); err != nil {
			// Start failed — rebuild from scratch.
			return Rebuild(ctx, containerID, image, workDir)
		}
		return containerID, nil

	case StateNotFound:
		return Rebuild(ctx, "", image, workDir)

	default:
		// Unknown state — rebuild.
		return Rebuild(ctx, containerID, image, workDir)
	}
}
