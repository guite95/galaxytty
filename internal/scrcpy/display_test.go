package scrcpy

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type recordingRunner struct {
	args   []string
	output []byte
	err    error
}

func (r *recordingRunner) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	r.args = append([]string(nil), args...)
	return r.output, r.err
}

func TestInspectUsesVersionOnly(t *testing.T) {
	runner := &recordingRunner{output: []byte("scrcpy 4.1 <https://github.com/Genymobile/scrcpy>\n\nDependencies:\n")}
	info, err := inspect(context.Background(), "/synthetic/scrcpy", runner)
	if err != nil || info.Path != "/synthetic/scrcpy" || info.Version != "4.1" {
		t.Fatalf("info=%+v err=%v", info, err)
	}
	if !reflect.DeepEqual(runner.args, []string{"--version"}) {
		t.Fatalf("args=%q", runner.args)
	}
}

func TestInspectRejectsMissingExecutableAndCommandFailure(t *testing.T) {
	if _, err := Inspect(context.Background(), "/definitely/missing/scrcpy"); err == nil {
		t.Fatal("expected missing executable error")
	}
	if _, err := inspect(context.Background(), "/synthetic/scrcpy", &recordingRunner{err: errors.New("exit status 1")}); err == nil {
		t.Fatal("expected version error")
	}
}

func TestParseDisplayID(t *testing.T) {
	tests := []struct {
		name string
		log  string
		want int64
	}{
		{
			name: "stdout prefix",
			log:  "INFO: New display: 1080x1920/344 (id=18)",
			want: 18,
		},
		{
			name: "stderr prefix",
			log:  "[server] INFO: New display: 1080x1920/344 (id=18)",
			want: 18,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDisplayID(tt.log)
			if err != nil || got != tt.want {
				t.Fatalf("ParseDisplayID() = %d, %v; want %d, nil", got, err, tt.want)
			}
		})
	}
}

func TestParseDisplayIDRejectsUnrelatedNumericID(t *testing.T) {
	if _, err := ParseDisplayID("INFO: New display: previous session (id=7), size 1080x1920 (id=18)"); err == nil {
		t.Fatal("ParseDisplayID() error = nil; want error for an unrelated ID")
	}
}
