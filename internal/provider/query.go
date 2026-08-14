package provider

import (
	"context"
	"fmt"
	"strings"
)

type Query struct {
	URI          string
	Projection   []string
	Where        string
	Sort         string
	FreeFormLast string
}

type ProbeResult struct {
	Rows int
}

func (s *Store) query(ctx context.Context, query Query) ([]map[string]string, error) {
	if strings.TrimSpace(query.URI) == "" {
		return nil, fmt.Errorf("%w: provider URI is empty", ErrProviderOutput)
	}
	if len(query.Projection) == 0 {
		return nil, fmt.Errorf("%w: projection is empty", ErrProviderOutput)
	}

	args := []string{"content", "query", "--uri", query.URI, "--projection", strings.Join(query.Projection, ":")}
	if query.Where != "" {
		quoted, err := remoteClause(query.Where)
		if err != nil {
			return nil, err
		}
		args = append(args, "--where", quoted)
	}
	if query.Sort != "" {
		quoted, err := remoteClause(query.Sort)
		if err != nil {
			return nil, err
		}
		args = append(args, "--sort", quoted)
	}

	output, err := s.shell.Shell(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("query %s: %w", query.URI, err)
	}
	text := string(output)
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "permission denial"), strings.Contains(lower, "permission denied"), strings.Contains(lower, "securityexception"):
		return nil, fmt.Errorf("%w: %s", ErrProviderPermissionDenied, query.URI)
	case strings.Contains(text, "[ERROR]"):
		return nil, fmt.Errorf("%w: content query failed for %s", ErrProviderOutput, query.URI)
	case strings.TrimSpace(text) == "No result found.":
		return []map[string]string{}, nil
	case strings.TrimSpace(text) == "":
		return nil, fmt.Errorf("%w: empty output for %s", ErrProviderOutput, query.URI)
	}

	rows, err := ParseProjectedRows(text, query.Projection, query.FreeFormLast)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrProviderOutput, query.URI)
	}
	return rows, nil
}

func remoteClause(value string) (string, error) {
	if strings.ContainsAny(value, "\n\r`$\\\"") {
		return "", fmt.Errorf("%w: unsafe provider clause", ErrProviderOutput)
	}
	return `"` + value + `"`, nil
}

func (s *Store) Probe(ctx context.Context, uri string, projection []string) (ProbeResult, error) {
	rows, err := s.query(ctx, Query{URI: uri, Projection: projection, Sort: "_id DESC LIMIT 1"})
	if err != nil {
		return ProbeResult{}, err
	}
	return ProbeResult{Rows: len(rows)}, nil
}
