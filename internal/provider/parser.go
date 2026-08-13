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
	rowRE := regexp.MustCompile(`(?m)^Row: [0-9]+ `)
	starts := rowRE.FindAllStringIndex(input, -1)
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
		pos := first + 1
		for _, m := range matches {
			candidate := body[m[2]:m[3]]
			if !allowed[candidate] {
				continue
			}
			out[key] = body[pos:m[0]]
			key = candidate
			pos = m[1]
		}
		out[key] = body[pos:]
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
