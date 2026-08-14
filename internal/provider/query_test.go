package provider

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type fakeShell struct {
	args   []string
	output []byte
	err    error
}

func (f *fakeShell) Shell(_ context.Context, args ...string) ([]byte, error) {
	f.args = append([]string(nil), args...)
	return f.output, f.err
}

func TestQueryBuildsLiteralRemoteClauses(t *testing.T) {
	shell := &fakeShell{output: []byte("Row: 0 _id=43\n")}
	store := NewStore(shell)
	rows, err := store.query(context.Background(), Query{
		URI:        "content://sms",
		Projection: []string{"_id"},
		Where:      "_id > 42",
		Sort:       "_id DESC LIMIT 1",
	})
	if err != nil || len(rows) != 1 || rows[0]["_id"] != "43" {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	want := []string{
		"content", "query", "--uri", "content://sms",
		"--projection", "_id",
		"--where", "\"_id > 42\"",
		"--sort", "\"_id DESC LIMIT 1\"",
	}
	if !reflect.DeepEqual(shell.args, want) {
		t.Fatalf("args=%q want=%q", shell.args, want)
	}
}

func TestQueryTreatsNoResultAsEmpty(t *testing.T) {
	store := NewStore(&fakeShell{output: []byte("No result found.\n")})
	rows, err := store.query(context.Background(), Query{URI: "content://sms", Projection: []string{"_id"}})
	if err != nil || len(rows) != 0 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
}

func TestQueryMapsProviderErrorsWithoutRowData(t *testing.T) {
	for _, tc := range []struct {
		name   string
		output string
		want   error
	}{
		{name: "permission", output: "java.lang.SecurityException: Permission Denial: private-value", want: ErrProviderPermissionDenied},
		{name: "content error", output: "[ERROR] Unsupported argument: private-value", want: ErrProviderOutput},
		{name: "blank", output: "\n", want: ErrProviderOutput},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := NewStore(&fakeShell{output: []byte(tc.output)})
			_, err := store.query(context.Background(), Query{URI: "content://sms", Projection: []string{"_id"}})
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want=%v", err, tc.want)
			}
			if strings.Contains(err.Error(), "private-value") {
				t.Fatalf("provider data leaked in error: %v", err)
			}
		})
	}
}

func TestQueryMapsPermissionDeniedShellErrorWithoutDetails(t *testing.T) {
	store := NewStore(&fakeShell{err: errors.New("java.lang.SecurityException: Permission Denial: private-value")})
	_, err := store.query(context.Background(), Query{URI: "content://sms", Projection: []string{"_id"}})
	if !errors.Is(err, ErrProviderPermissionDenied) {
		t.Fatalf("err=%v", err)
	}
	if strings.Contains(err.Error(), "private-value") {
		t.Fatalf("provider data leaked in error: %v", err)
	}
}

func TestQueryRejectsUnsafeRemoteClause(t *testing.T) {
	store := NewStore(&fakeShell{})
	_, err := store.query(context.Background(), Query{
		URI: "content://sms", Projection: []string{"_id"}, Where: "_id > 1\nwhoami",
	})
	if !errors.Is(err, ErrProviderOutput) {
		t.Fatalf("err=%v", err)
	}
}

func TestProbeUsesNonSensitiveProjectionAndCountsRows(t *testing.T) {
	shell := &fakeShell{output: []byte("Row: 0 _id=43\n")}
	store := NewStore(shell)
	result, err := store.Probe(context.Background(), "content://sms", []string{"_id"})
	if err != nil || result.Rows != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	want := []string{
		"content", "query", "--uri", "content://sms",
		"--projection", "_id",
		"--sort", "\"_id DESC LIMIT 1\"",
	}
	if !reflect.DeepEqual(shell.args, want) {
		t.Fatalf("args=%q want=%q", shell.args, want)
	}
}
