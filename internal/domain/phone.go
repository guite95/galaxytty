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
	if strings.HasPrefix(b.String(), "+82") {
		return "0" + strings.TrimPrefix(b.String(), "+82")
	}
	return b.String()
}
