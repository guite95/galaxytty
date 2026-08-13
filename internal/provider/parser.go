// Package provider contains Android Content Provider adapter primitives.
package provider

import (
	"fmt"
	"regexp"
	"strings"
)

var fieldBoundary = regexp.MustCompile(`, ([A-Za-z_][A-Za-z0-9_]*)=`)

// ParseContentRows parses projected `content query` output. Values may contain commas,
// equals signs, Unicode and continuation newlines. Projection column names delimit fields.
func ParseContentRows(input string, columns []string) ([]map[string]string, error) {
	freeForm := ""
	if len(columns) > 0 && columns[len(columns)-1] == "body" {
		freeForm = "body"
	}
	return ParseProjectedRows(input, columns, freeForm)
}

// ParseProjectedRows parses an explicit projection. If freeFormLast is set it
// must be the final column; its value consumes the remainder of the row verbatim.
func ParseProjectedRows(input string, columns []string, freeFormLast string) ([]map[string]string, error) {
	if len(columns) == 0 {
		return nil, fmt.Errorf("projection is empty")
	}
	if freeFormLast != "" && columns[len(columns)-1] != freeFormLast {
		return nil, fmt.Errorf("free-form column must be last")
	}
	rowRE := regexp.MustCompile(`(?m)^Row: [0-9]+ `)
	starts := rowRE.FindAllStringIndex(input, -1)
	if len(starts) == 0 && strings.TrimSpace(input) != "" {
		return nil, fmt.Errorf("malformed content output: no rows")
	}
	rows := make([]map[string]string, 0, len(starts))
	for i, s := range starts {
		end := len(input)
		if i+1 < len(starts) {
			end = starts[i+1][0]
		}
		body := strings.TrimSuffix(input[s[1]:end], "\n")
		allowed := map[string]bool{}
		for _, c := range columns {
			allowed[c] = true
		}
		matches := fieldBoundary.FindAllStringSubmatchIndex(body, -1)
		first := strings.Index(body, "=")
		if first < 1 {
			return nil, fmt.Errorf("malformed content row")
		}
		out := map[string]string{}
		key := body[:first]
		if key != columns[0] {
			return nil, fmt.Errorf("malformed projected row: expected first column %q", columns[0])
		}
		pos := first + 1
		columnIndex := 0
		for _, m := range matches {
			candidate := body[m[2]:m[3]]
			if freeFormLast != "" && key == freeFormLast {
				break
			}
			if !allowed[candidate] || columnIndex+1 >= len(columns) || candidate != columns[columnIndex+1] {
				continue
			}
			out[key] = body[pos:m[0]]
			key = candidate
			pos = m[1]
			columnIndex++
		}
		out[key] = body[pos:]
		if len(out) != len(columns) {
			return nil, fmt.Errorf("malformed projected row: expected %d columns, got %d", len(columns), len(out))
		}
		rows = append(rows, out)
	}
	return rows, nil
}

func DirectionFromAndroid(v int) (uint8, error) {
	if v == 1 || v == 2 {
		return uint8(v), nil
	}
	return 0, fmt.Errorf("unsupported Android message type: %d", v)
}
