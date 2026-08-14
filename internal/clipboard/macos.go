package clipboard

import (
	"bytes"
	"context"
	"io"
	"os/exec"

	"github.com/galaxytty/galaxytty/internal/domain"
)

type runner interface {
	Run(context.Context, string, io.Reader) ([]byte, error)
}

type commandRunner struct{}

func (commandRunner) Run(ctx context.Context, name string, stdin io.Reader) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name)
	cmd.Stdin = stdin
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	return stdout.Bytes(), err
}

type Mac struct {
	copyPath, pastePath string
	runner              runner
}

func New(copyPath, pastePath string) *Mac {
	if copyPath == "" {
		copyPath = "pbcopy"
	}
	if pastePath == "" {
		pastePath = "pbpaste"
	}
	return &Mac{copyPath: copyPath, pastePath: pastePath, runner: commandRunner{}}
}

func (m *Mac) Read(ctx context.Context) (string, error) {
	output, err := m.runner.Run(ctx, m.pastePath, nil)
	if err != nil {
		return "", safeClipboardError{kind: domain.ErrClipboardRead, cause: err}
	}
	return string(output), nil
}

func (m *Mac) Set(ctx context.Context, value string) error {
	_, err := m.runner.Run(ctx, m.copyPath, bytes.NewBufferString(value))
	if err != nil {
		return safeClipboardError{kind: domain.ErrClipboardSet, cause: err}
	}
	return nil
}

type safeClipboardError struct {
	kind  error
	cause error
}

func (e safeClipboardError) Error() string { return e.kind.Error() }
func (e safeClipboardError) Unwrap() []error {
	return []error{e.kind, e.cause}
}

var _ domain.Clipboard = (*Mac)(nil)
