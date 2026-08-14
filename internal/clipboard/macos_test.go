package clipboard

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/galaxytty/galaxytty/internal/domain"
)

type runnerCall struct {
	name  string
	stdin string
}

type recordingRunner struct {
	calls  []runnerCall
	output []byte
	err    error
}

func (r *recordingRunner) Run(_ context.Context, name string, stdin io.Reader) ([]byte, error) {
	value := ""
	if stdin != nil {
		contents, _ := io.ReadAll(stdin)
		value = string(contents)
	}
	r.calls = append(r.calls, runnerCall{name: name, stdin: value})
	return r.output, r.err
}

func TestMacReadsAndSetsExactUnicodeClipboard(t *testing.T) {
	runner := &recordingRunner{output: []byte("기존 😀\n")}
	clip := New("/synthetic/pbcopy", "/synthetic/pbpaste")
	clip.runner = runner

	old, err := clip.Read(context.Background())
	if err != nil || old != "기존 😀\n" {
		t.Fatalf("old=%q err=%v", old, err)
	}
	runner.output = nil
	if err := clip.Set(context.Background(), "안녕, Galaxy 😀"); err != nil {
		t.Fatal(err)
	}
	want := []runnerCall{
		{name: "/synthetic/pbpaste"},
		{name: "/synthetic/pbcopy", stdin: "안녕, Galaxy 😀"},
	}
	if len(runner.calls) != len(want) {
		t.Fatalf("calls=%+v", runner.calls)
	}
	for index := range want {
		if runner.calls[index] != want[index] {
			t.Fatalf("call[%d]=%+v want=%+v", index, runner.calls[index], want[index])
		}
	}
}

func TestMacPreservesEmptyClipboard(t *testing.T) {
	runner := &recordingRunner{}
	clip := New("pbcopy", "pbpaste")
	clip.runner = runner
	value, err := clip.Read(context.Background())
	if err != nil || value != "" {
		t.Fatalf("value=%q err=%v", value, err)
	}
}

func TestMacMapsReadAndSetErrorsWithoutClipboardContents(t *testing.T) {
	const secret = "private clipboard value"
	for _, tc := range []struct {
		name string
		run  func(*Mac) error
		want error
	}{
		{"read", func(c *Mac) error { _, err := c.Read(context.Background()); return err }, domain.ErrClipboardRead},
		{"set", func(c *Mac) error { return c.Set(context.Background(), secret) }, domain.ErrClipboardSet},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := &recordingRunner{err: errors.New("synthetic process failure")}
			clip := New("pbcopy", "pbpaste")
			clip.runner = runner
			err := tc.run(clip)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want=%v", err, tc.want)
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("error exposes clipboard: %q", err)
			}
		})
	}
}

func TestMacDefersMissingExecutableFailureUntilUse(t *testing.T) {
	clip := New("/definitely/missing/pbcopy", "/definitely/missing/pbpaste")
	if _, err := clip.Read(context.Background()); !errors.Is(err, domain.ErrClipboardRead) {
		t.Fatalf("Read() error=%v", err)
	}
	if err := clip.Set(context.Background(), "synthetic"); !errors.Is(err, domain.ErrClipboardSet) {
		t.Fatalf("Set() error=%v", err)
	}
}
