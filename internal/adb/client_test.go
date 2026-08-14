package adb

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

type recordingRunner struct {
	name   string
	args   []string
	stdout []byte
	stderr []byte
	err    error
	wait   bool
}

func (r *recordingRunner) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	r.name = name
	r.args = append([]string(nil), args...)
	if r.wait {
		<-ctx.Done()
		return nil, nil, ctx.Err()
	}
	return r.stdout, r.stderr, r.err
}

func TestShellUsesSelectedTargetWithoutHostShell(t *testing.T) {
	runner := &recordingRunner{stdout: []byte("samsung\n")}
	client := newClient("/synthetic/adb", time.Second, runner)
	got, err := client.Shell(context.Background(), "synthetic-target", "getprop", "ro.product.manufacturer")
	if err != nil || string(got) != "samsung\n" {
		t.Fatalf("got=%q err=%v", got, err)
	}
	want := []string{"-s", "synthetic-target", "shell", "getprop", "ro.product.manufacturer"}
	if runner.name != "/synthetic/adb" || !reflect.DeepEqual(runner.args, want) {
		t.Fatalf("name=%q args=%q want=%q", runner.name, runner.args, want)
	}
}

func TestVersionCapturesStdout(t *testing.T) {
	runner := &recordingRunner{stdout: []byte("Android Debug Bridge version 1.0.41\n")}
	client := newClient("/synthetic/adb", time.Second, runner)
	got, err := client.Version(context.Background())
	if err != nil || got != "Android Debug Bridge version 1.0.41" {
		t.Fatalf("got=%q err=%v", got, err)
	}
	if !reflect.DeepEqual(runner.args, []string{"version"}) {
		t.Fatalf("args=%q", runner.args)
	}
}

func TestNewClientRejectsMissingExecutable(t *testing.T) {
	_, err := NewClient("/definitely/missing/adb", time.Second)
	if !errors.Is(err, ErrADBNotFound) {
		t.Fatalf("err=%v", err)
	}
}

func TestClientMapsADBStateErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stderr string
		want   error
	}{
		{name: "unauthorized", stderr: "error: device unauthorized", want: ErrUnauthorized},
		{name: "offline", stderr: "error: device offline", want: ErrOffline},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := &recordingRunner{stderr: []byte(tc.stderr), err: errors.New("exit status 1")}
			client := newClient("/synthetic/adb", time.Second, runner)
			_, err := client.Shell(context.Background(), "synthetic-target", "getprop", "ro.serialno")
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want=%v", err, tc.want)
			}
		})
	}
}

func TestClientPreservesSafeStderr(t *testing.T) {
	runner := &recordingRunner{stderr: []byte("synthetic failure"), err: errors.New("exit status 1")}
	client := newClient("/synthetic/adb", time.Second, runner)
	_, err := client.Version(context.Background())
	if err == nil || !strings.Contains(err.Error(), "synthetic failure") {
		t.Fatalf("err=%v", err)
	}
}

func TestClientReturnsContextErrors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		ctx     func() (context.Context, context.CancelFunc)
		timeout time.Duration
		want    error
	}{
		{
			name: "canceled",
			ctx: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx, func() {}
			},
			timeout: time.Second,
			want:    context.Canceled,
		},
		{
			name: "deadline",
			ctx: func() (context.Context, context.CancelFunc) {
				return context.WithCancel(context.Background())
			},
			timeout: time.Millisecond,
			want:    context.DeadlineExceeded,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := tc.ctx()
			defer cancel()
			client := newClient("/synthetic/adb", tc.timeout, &recordingRunner{wait: true})
			_, err := client.Version(ctx)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want=%v", err, tc.want)
			}
		})
	}
}
