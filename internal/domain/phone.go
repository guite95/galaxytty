package domain

import "strings"

func NormalizePhone(v string) string {
	v = strings.TrimSpace(v)
	var b strings.Builder
	for i, r := range v {
		if r >= '0' && r <= '9' || (r == '+' && i == 0) {
			b.WriteRune(r)
		}
	}
	normalized := b.String()
	if strings.HasPrefix(normalized, "+82") {
		national := strings.TrimPrefix(normalized, "+82")
		national = strings.TrimPrefix(national, "0")
		if national == "" {
			return ""
		}
		return "0" + national
	}
	return normalized
}
