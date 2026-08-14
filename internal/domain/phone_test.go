package domain

import "testing"

func TestNormalizePhoneFormats(t *testing.T) {
	tests := map[string]string{
		"010-1234-5678":       "01012345678",
		"+82 10-1234-5678":    "01012345678",
		"+82 (0)10-1234-5678": "01012345678",
		" 1588-0000 ":         "15880000",
	}
	for input, want := range tests {
		if got := NormalizePhone(input); got != want {
			t.Fatalf("NormalizePhone(%q)=%q want=%q", input, got, want)
		}
	}
}
