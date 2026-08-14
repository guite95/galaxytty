package scrcpy

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var displayIDRE = regexp.MustCompile(`New display:\s+[0-9]+x[0-9]+(?:/[0-9]+)?\s+\(id=([0-9]+)\)`)
var versionRE = regexp.MustCompile(`(?m)^scrcpy ([0-9]+(?:\.[0-9]+)*)\b`)

type VersionInfo struct {
	Path    string
	Version string
}

type runner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	return output.Bytes(), err
}

func ParseDisplayID(log string) (int64, error) {
	m := displayIDRE.FindStringSubmatch(log)
	if m == nil {
		return 0, fmt.Errorf("display ID not found")
	}
	return strconv.ParseInt(m[1], 10, 64)
}

func Inspect(ctx context.Context, path string) (VersionInfo, error) {
	if path == "" {
		path = "scrcpy"
	}
	resolved, err := exec.LookPath(path)
	if err != nil {
		return VersionInfo{}, fmt.Errorf("scrcpy executable not found")
	}
	return inspect(ctx, resolved, execRunner{})
}

func inspect(ctx context.Context, path string, commandRunner runner) (VersionInfo, error) {
	output, err := commandRunner.Run(ctx, path, "--version")
	if err != nil {
		return VersionInfo{}, fmt.Errorf("inspect scrcpy version: %w", err)
	}
	match := versionRE.FindStringSubmatch(strings.TrimSpace(string(output)))
	if match == nil {
		return VersionInfo{}, fmt.Errorf("scrcpy version not found")
	}
	return VersionInfo{Path: path, Version: match[1]}, nil
}
