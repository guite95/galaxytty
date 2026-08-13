package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	e := Execute(context.Background(), strings.NewReader(""), &out, args)
	return out.String(), e
}
func TestHelp(t *testing.T) {
	v, e := run(t, "--help")
	if e != nil || !strings.Contains(v, "Usage:") {
		t.Fatal(v, e)
	}
}
func TestMockCommands(t *testing.T) {
	for _, a := range [][]string{{"conversations", "--mock"}, {"unread", "--mock"}, {"messages", "1", "--mock"}, {"doctor", "--mock"}, {"send", "--mock", "--to", "01012345678", "--text", "hello"}} {
		if _, e := run(t, a...); e != nil {
			t.Fatalf("%v: %v", a, e)
		}
	}
}
func TestJSONOnly(t *testing.T) {
	for _, a := range [][]string{{"conversations", "--mock", "--json"}, {"unread", "--mock", "--json"}, {"messages", "1", "--mock", "--json"}} {
		v, e := run(t, a...)
		if e != nil || !json.Valid([]byte(v)) {
			t.Fatalf("%v: %q %v", a, v, e)
		}
	}
}
func TestSendValidation(t *testing.T) {
	if _, e := run(t, "send", "--mock", "--to", "010"); e == nil {
		t.Fatal("expected text validation")
	}
}
