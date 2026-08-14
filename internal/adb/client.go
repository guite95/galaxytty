package adb

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type runner interface {
	Run(context.Context, []byte, string, ...string) ([]byte, []byte, error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, input []byte, name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

type Client struct {
	path    string
	timeout time.Duration
	runner  runner
}

type CommandError struct {
	Args   []string
	Stderr string
	Err    error
}

func (e *CommandError) Error() string {
	detail := strings.TrimSpace(e.Stderr)
	if detail == "" {
		return fmt.Sprintf("adb %s: %v", strings.Join(e.Args, " "), e.Err)
	}
	return fmt.Sprintf("adb %s: %s: %v", strings.Join(e.Args, " "), detail, e.Err)
}

func (e *CommandError) Unwrap() error { return e.Err }

func NewClient(path string, timeout time.Duration) (*Client, error) {
	if path == "" {
		path = "adb"
	}
	resolved, err := exec.LookPath(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrADBNotFound, path)
	}
	return newClient(resolved, timeout, execRunner{}), nil
}

func newClient(path string, timeout time.Duration, commandRunner runner) *Client {
	return &Client{path: path, timeout: timeout, runner: commandRunner}
}

func (c *Client) Version(ctx context.Context) (string, error) {
	stdout, err := c.run(ctx, "version")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(stdout)), nil
}

func (c *Client) Shell(ctx context.Context, target string, args ...string) ([]byte, error) {
	commandArgs := make([]string, 0, len(args)+3)
	commandArgs = append(commandArgs, "-s", target, "shell")
	commandArgs = append(commandArgs, args...)
	return c.run(ctx, commandArgs...)
}

func (c *Client) ShellStdin(ctx context.Context, target, command string) ([]byte, error) {
	if strings.TrimSpace(command) == "" || strings.IndexByte(command, 0) >= 0 {
		return nil, errors.New("invalid remote shell command")
	}
	return c.runCommand(ctx, []byte(command), true, "-s", target, "shell")
}

func (c *Client) run(ctx context.Context, args ...string) ([]byte, error) {
	return c.runCommand(ctx, nil, false, args...)
}

func (c *Client) runCommand(ctx context.Context, input []byte, sensitive bool, args ...string) ([]byte, error) {
	runCtx := ctx
	cancel := func() {}
	if c.timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, c.timeout)
	}
	defer cancel()

	stdout, stderr, err := c.runner.Run(runCtx, input, c.path, args...)
	if runCtx.Err() != nil {
		return nil, runCtx.Err()
	}
	if err == nil {
		return stdout, nil
	}

	detail := strings.ToLower(string(stderr))
	switch {
	case strings.Contains(detail, "unauthorized"):
		return nil, fmt.Errorf("%w: %v", ErrUnauthorized, err)
	case strings.Contains(detail, "offline"):
		return nil, fmt.Errorf("%w: %v", ErrOffline, err)
	case strings.Contains(detail, "no devices/emulators found"):
		return nil, fmt.Errorf("%w: %v", ErrNoDevices, err)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, err
	}
	if sensitive {
		return nil, &CommandError{Args: append([]string(nil), args...), Err: err}
	}
	return nil, &CommandError{Args: append([]string(nil), args...), Stderr: string(stderr), Err: err}
}
